package application

import (
	"cave-archive/internal/checker"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

type differenceCacheEntry struct {
	revisionID string
	report     DifferenceReport
}

type Service struct {
	Store *store.Store

	differenceMu    sync.Mutex
	differenceCache map[string]differenceCacheEntry
}

func New(s *store.Store) *Service {
	return &Service{Store: s, differenceCache: map[string]differenceCacheEntry{}}
}
func (s *Service) Create(code, cave, date, datum, key string) (*domain.Archive, error) {
	a, e := domain.NewArchive(code, cave, date, datum)
	if e != nil {
		return nil, e
	}
	digest := domain.ArchiveRequestDigest(code, cave, date, datum)
	a, _, e = s.Store.CreateChecked(a, strings.TrimSpace(key), digest)
	if e != nil {
		return nil, e
	}
	return a, nil
}
func (s *Service) AddRevision(id string, r *domain.Revision, expected int) (*domain.Archive, error) {
	return s.AddRevisionIdempotent(id, r, expected, "")
}
func (s *Service) AddRevisionIdempotent(id string, r *domain.Revision, expected int, key string) (*domain.Archive, error) {
	if expected <= 0 {
		return nil, fmt.Errorf("%w: expectedVersion不能为空", domain.ErrInvalid)
	}
	if r == nil {
		return nil, fmt.Errorf("%w: 修订不能为空", domain.ErrInvalid)
	}
	key = strings.TrimSpace(key)
	digest := domain.RevisionRequestDigest(id, expected, r)
	contentHash := domain.RevisionContentHash(r)
	var revisionID string
	a, err := s.Store.Transact(id, "revision_created", func(candidate *domain.Archive) error {
		if key != "" {
			for _, existing := range candidate.Revisions {
				if existing.IdempotencyKey != key {
					continue
				}
				if existing.RequestDigest != digest {
					return &domain.BusinessError{Cause: domain.ErrIdempotencyConflict, Code: "idempotency_conflict", Message: "同一idempotencyKey对应的修订参数不同", ExistingRevisionID: existing.ID}
				}
				revisionID = existing.ID
				return store.ErrNoChange
			}
		}
		if candidate.Version != expected {
			return fmt.Errorf("%w: 当前版本为%d", domain.ErrConflict, candidate.Version)
		}
		for _, existing := range candidate.Revisions {
			if existing.ContentHash == contentHash {
				return &domain.BusinessError{Cause: domain.ErrDuplicateContent, Code: "duplicate_revision_content", Message: "草稿中已存在相同内容的修订", ExistingRevisionID: existing.ID}
			}
		}
		r.IdempotencyKey, r.RequestDigest = key, digest
		if err := candidate.AddRevision(r, expected); err != nil {
			return err
		}
		revisionID = r.ID
		return nil
	})
	if err != nil {
		return nil, err
	}
	if stored := a.Revisions[revisionID]; stored != nil {
		*r = *stored
	}
	return a, nil
}
func (s *Service) ValidateRevision(r *domain.Revision) domain.RevisionValidation {
	return domain.ValidateRevision(r)
}
func (s *Service) Submit(id string, expected int) (*domain.Archive, error) {
	return s.Store.Transact(id, "submitted", func(a *domain.Archive) error { return a.Submit(expected) })
}
func (s *Service) Check(id string, expected int) (*checker.Result, error) {
	return s.CheckVersion(id, expected, "topology-v1")
}
func (s *Service) CheckVersion(id string, expected int, ruleSetVersion string) (*checker.Result, error) {
	if ruleSetVersion == "" {
			ruleSetVersion = checker.RuleSetTopologyV1
	}
	var result *checker.Result
	_, err := s.Store.Transact(id, "check_run", func(a *domain.Archive) error {
		r := a.CurrentRevision()
		if r == nil {
			return fmt.Errorf("%w: 无当前修订", domain.ErrInvalid)
		}
		inputHash := domain.CheckInputHash(r, ruleSetVersion)
		for _, run := range a.CheckRuns {
			if run.RevisionID != r.ID || run.RuleSetVersion != ruleSetVersion || run.InputHash != inputHash {
				continue
			}
			findings := findingsForRun(a, run)
			if !runConsistent(a, run, findings) {
				return &domain.BusinessError{Cause: domain.ErrDigestMismatch, Code: "check_run_digest_mismatch", Message: "质检运行摘要不一致"}
			}
			copy := *run
			copy.Consistent = true
			result = &checker.Result{CheckRunID: copy.ID, Run: &copy, Findings: findings, Reused: true}
			return store.ErrNoChange
		}
		if expected > 0 && a.Version != expected {
			return fmt.Errorf("%w: 当前版本为%d", domain.ErrConflict, a.Version)
		}
		if a.Status != domain.StatusPendingCheck {
			return fmt.Errorf("%w: 仅当前待质检修订可检查", domain.ErrForbidden)
		}
		generated, e := checker.RunVersion(a, r, ruleSetVersion)
		if e != nil {
			return e
		}
		if e = a.StartReview(generated.Run, generated.Findings, expected); e != nil {
			return e
		}
		result = generated
		return nil
	})
	return result, err
}

func findingsForRun(a *domain.Archive, run *domain.CheckRun) []*domain.Finding {
	out := make([]*domain.Finding, 0, len(run.FindingIDs))
	for _, id := range run.FindingIDs {
		if finding := a.Findings[id]; finding != nil {
			out = append(out, finding)
		}
	}
	return out
}

func runConsistent(a *domain.Archive, run *domain.CheckRun, findings []*domain.Finding) bool {
	revision := a.Revisions[run.RevisionID]
	return revision != nil && domain.CheckInputHash(revision, run.RuleSetVersion) == run.InputHash && domain.CheckRunConsistent(run, findings)
}

func (s *Service) CheckRun(id, runID string) (*domain.CheckRun, error) {
	a, err := s.Store.Get(id)
	if err != nil {
		return nil, err
	}
	run := a.CheckRuns[runID]
	if run == nil {
		return nil, domain.ErrNotFound
	}
	if !runConsistent(a, run, findingsForRun(a, run)) {
		return nil, &domain.BusinessError{Cause: domain.ErrDigestMismatch, Code: "check_run_digest_mismatch", Message: "质检运行摘要不一致"}
	}
	run.Consistent = true
	return run, nil
}
func (s *Service) Decide(id, fid, decision, reason, reviewer string, expected int) (*domain.Finding, error) {
	return s.DecideDetailed(id, domain.DecisionInput{FindingID: fid, Decision: decision, Reason: reason, Reviewer: reviewer, Rectification: reason}, expected)
}
func (s *Service) DecideDetailed(id string, input domain.DecisionInput, expected int) (*domain.Finding, error) {
	a, e := s.Store.Transact(id, "finding_decided", func(candidate *domain.Archive) error {
		if input.Decision == "rectify" && input.RelatedObject == "" {
			if f := candidate.Findings[input.FindingID]; f != nil {
				input.RelatedObject = f.SubjectType + ":" + f.SubjectID
			}
		}
		return candidate.BatchDecide([]domain.DecisionInput{input}, expected)
	})
	if e != nil {
		return nil, e
	}
	return a.Findings[input.FindingID], nil
}
func (s *Service) BatchDecide(id string, items []domain.DecisionInput, expected int) (*domain.Archive, error) {
	return s.Store.Transact(id, "findings_decided", func(a *domain.Archive) error { return a.BatchDecide(items, expected) })
}
func (s *Service) ReviewPreview(id string) (domain.ReviewGate, error) {
	a, e := s.Store.Get(id)
	if e != nil {
		return domain.ReviewGate{}, e
	}
	return a.ReviewPreview(), nil
}
func (s *Service) CompleteReview(id string, expected int) (*domain.Archive, error) {
	return s.Store.Transact(id, "review_completed", func(a *domain.Archive) error { return a.FinalizeReview(expected) })
}
func (s *Service) Freeze(id, by string, expected int) (*domain.Manifest, error) {
	var manifest *domain.Manifest
	_, e := s.Store.Transact(id, "frozen", func(a *domain.Archive) error {
		var err error
		manifest, err = a.Freeze(expected, by)
		return err
	})
	return manifest, e
}
func (s *Service) FreezeChecked(id, by string, expected, previewVersion int, previewHash string) (*domain.Manifest, error) {
	var manifest *domain.Manifest
	_, e := s.Store.Transact(id, "frozen", func(a *domain.Archive) error {
		var err error
		manifest, err = a.FreezeWithPreview(expected, previewVersion, previewHash, by)
		return err
	})
	return manifest, e
}
func (s *Service) FreezePreview(id string) (*domain.FreezePreview, error) {
	a, e := s.Store.Get(id)
	if e != nil {
		return nil, e
	}
	return a.PreviewFreeze()
}
func (s *Service) Issue(id, by string, expected int) (*domain.Certificate, error) {
	return s.IssueIdempotent(id, by, expected, "legacy:"+id)
}
func (s *Service) IssueIdempotent(id, by string, expected int, key string) (*domain.Certificate, error) {
	key, by = strings.TrimSpace(key), strings.TrimSpace(by)
	if key == "" {
		return nil, fmt.Errorf("%w: idempotencyKey不能为空", domain.ErrInvalid)
	}
	var certificate *domain.Certificate
	_, err := s.Store.Transact(id, "accepted", func(a *domain.Archive) error {
		manifestID, manifestHash := "", ""
		if a.Manifest != nil {
			manifestID, manifestHash = a.Manifest.ManifestID, a.Manifest.ManifestHash
		}
		digest := domain.Hash(struct{ ArchiveID, IssuedBy, Key, ManifestID, ManifestHash string }{id, by, key, manifestID, manifestHash})
		if a.Certificate != nil {
			if a.Certificate.IdempotencyKey == key && a.Certificate.RequestDigest == digest {
				copy := *a.Certificate
				certificate = &copy
				return store.ErrNoChange
			}
			return &domain.BusinessError{Cause: domain.ErrIdempotencyConflict, Code: "certificate_already_issued", Message: "归档验收凭据已由其他签发请求生成"}
		}
		created, e := a.Issue(by, expected)
		if e != nil {
			return e
		}
		created.IdempotencyKey, created.RequestDigest = key, digest
		created.CertificateHash = ""
		created.CertificateHash = domain.Hash(created)
		copy := *created
		certificate = &copy
		return nil
	})
	return certificate, err
}
func (s *Service) Detail(id string) (*domain.Archive, error) {
	a, err := s.Store.Get(id)
	if err != nil {
		return nil, err
	}
	for _, run := range a.CheckRuns {
		if !runConsistent(a, run, findingsForRun(a, run)) {
			return nil, &domain.BusinessError{Cause: domain.ErrDigestMismatch, Code: "check_run_digest_mismatch", Message: "质检运行摘要不一致"}
		}
		run.Consistent = true
	}
	return a, nil
}
func (s *Service) List() []*domain.Archive {
	out := s.Store.List()
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

type ArchiveQuery struct {
	Keyword, Status, From, To string
	Page, PageSize            int
}
type ArchivePage struct {
	Archives     []*domain.Archive     `json:"archives"`
	Total        int                   `json:"total"`
	Page         int                   `json:"page"`
	PageSize     int                   `json:"pageSize"`
	StatusCounts map[domain.Status]int `json:"statusCounts"`
	Summary      *ArchiveSummary       `json:"summary,omitempty"`
}

type ArchiveSummary struct {
	GeneratedAt                       string                `json:"generatedAt"`
	StatusCounts                      map[domain.Status]int `json:"statusCounts"`
	ArchiveCount                      int                   `json:"archiveCount"`
	RevisionCount                     int                   `json:"revisionCount"`
	StationCount                      int                   `json:"stationCount"`
	LegCount                          int                   `json:"legCount"`
	CertificateCount                  int                   `json:"certificateCount"`
	ValidCertificateCount             int                   `json:"validCertificateCount"`
	MissingCertificateCount           int                   `json:"missingCertificateCount"`
	InconsistentCertificateCount      int                   `json:"inconsistentCertificateCount"`
	UnresolvedFindingCount            int                   `json:"unresolvedFindingCount"`
	RectificationCount                int                   `json:"rectificationCount"`
	FrozenDigestAnomalyCount          int                   `json:"frozenDigestAnomalyCount"`
	UnresolvedArchiveIDs              []string              `json:"unresolvedArchiveIds"`
	RectificationArchiveIDs           []string              `json:"rectificationArchiveIds"`
	FrozenDigestAnomalyArchiveIDs     []string              `json:"frozenDigestAnomalyArchiveIds"`
	ValidCertificateArchiveIDs        []string              `json:"validCertificateArchiveIds"`
	MissingCertificateArchiveIDs      []string              `json:"missingCertificateArchiveIds"`
	InconsistentCertificateArchiveIDs []string              `json:"inconsistentCertificateArchiveIds"`
}

func (s *Service) Search(q ArchiveQuery) (ArchivePage, error) {
	if len(q.Keyword) > 100 {
		return ArchivePage{}, fmt.Errorf("%w: keyword不能超过100个字符", domain.ErrInvalid)
	}
	if q.Status != "" {
		ok := false
		for _, st := range []domain.Status{domain.StatusDraft, domain.StatusPendingCheck, domain.StatusReviewing, domain.StatusRework, domain.StatusFreezable, domain.StatusFrozen, domain.StatusAccepted} {
			if q.Status == string(st) {
				ok = true
			}
		}
		if !ok {
			return ArchivePage{}, fmt.Errorf("%w: status字段值非法", domain.ErrInvalid)
		}
	}
	if q.From != "" && q.To != "" && q.To < q.From {
		return ArchivePage{}, fmt.Errorf("%w: to不能早于from", domain.ErrInvalid)
	}
	for field, value := range map[string]string{"from": strings.TrimSpace(q.From), "to": strings.TrimSpace(q.To)} {
		if value != "" {
			if _, err := time.Parse("2006-01-02", value); err != nil {
				return ArchivePage{}, &domain.BusinessError{Cause: domain.ErrInvalid, Code: "invalid_date", Message: field + "日期格式必须为YYYY-MM-DD", Field: field}
			}
		}
	}
	from, to := strings.TrimSpace(q.From), strings.TrimSpace(q.To)
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	all := s.Store.List()
	sort.Slice(all, func(i, j int) bool {
		if all[i].UpdatedAt == all[j].UpdatedAt {
			return all[i].ID < all[j].ID
		}
		return all[i].UpdatedAt > all[j].UpdatedAt
	})
	out := make([]*domain.Archive, 0)
	counts := map[domain.Status]int{}
	for _, st := range []domain.Status{domain.StatusDraft, domain.StatusPendingCheck, domain.StatusReviewing, domain.StatusRework, domain.StatusFreezable, domain.StatusFrozen, domain.StatusAccepted} {
		counts[st] = 0
	}
	for _, a := range all {
		if kw != "" && !strings.Contains(strings.ToLower(strings.TrimSpace(a.ArchiveCode)), kw) && !strings.Contains(strings.ToLower(strings.TrimSpace(a.CaveName)), kw) {
			continue
		}
		if q.Status != "" && string(a.Status) != q.Status {
			continue
		}
		if from != "" && a.SurveyDate < from {
			continue
		}
		if to != "" && a.SurveyDate > to {
			continue
		}
		counts[a.Status]++
		out = append(out, a)
	}
	total := len(out)
	page := q.Page
	if page < 1 {
		page = 1
	}
	size := q.PageSize
	if size < 1 {
		size = 20
	}
	if size > 100 {
		size = 100
	}
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	return ArchivePage{Archives: out[start:end], Total: total, Page: page, PageSize: size, StatusCounts: counts}, nil
}

func (s *Service) Summary(q ArchiveQuery) (ArchiveSummary, error) {
	q.Page, q.PageSize = 1, 100
	page, err := s.Search(q)
	if err != nil {
		return ArchiveSummary{}, err
	}
	matched := make([]*domain.Archive, 0, page.Total)
	q.PageSize = 100
	for p := 1; len(matched) < page.Total; p++ {
		q.Page = p
		chunk, e := s.Search(q)
		if e != nil {
			return ArchiveSummary{}, e
		}
		matched = append(matched, chunk.Archives...)
		if len(chunk.Archives) == 0 {
			break
		}
	}
	out := ArchiveSummary{GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano), StatusCounts: page.StatusCounts, ArchiveCount: len(matched)}
	for _, a := range matched {
		out.RevisionCount += len(a.Revisions)
		for _, revision := range a.Revisions {
			out.StationCount += len(revision.Stations)
			out.LegCount += len(revision.Legs)
		}
		gate := a.ReviewPreview()
		if gate.Unresolved > 0 || gate.HighRiskPending > 0 {
			out.UnresolvedFindingCount += gate.Unresolved + gate.HighRiskPending
			out.UnresolvedArchiveIDs = append(out.UnresolvedArchiveIDs, a.ID)
		}
		count := 0
		for _, finding := range a.Findings {
			if finding.Decision == "rectify" && finding.TraceStatus != "resolved" {
				count++
			}
		}
		if count > 0 {
			out.RectificationCount += count
			out.RectificationArchiveIDs = append(out.RectificationArchiveIDs, a.ID)
		}
		if a.Manifest != nil && !domain.ManifestMatchesRevision(a.Manifest, a.Revisions[a.Manifest.RevisionID]) {
			out.FrozenDigestAnomalyCount++
			out.FrozenDigestAnomalyArchiveIDs = append(out.FrozenDigestAnomalyArchiveIDs, a.ID)
		}
		if a.Certificate == nil {
			out.MissingCertificateCount++
			out.MissingCertificateArchiveIDs = append(out.MissingCertificateArchiveIDs, a.ID)
			continue
		}
		out.CertificateCount++
		if a.VerifyCertificate().Result == "valid" {
			out.ValidCertificateCount++
			out.ValidCertificateArchiveIDs = append(out.ValidCertificateArchiveIDs, a.ID)
		} else {
			out.InconsistentCertificateCount++
			out.InconsistentCertificateArchiveIDs = append(out.InconsistentCertificateArchiveIDs, a.ID)
		}
	}
	sort.Strings(out.UnresolvedArchiveIDs)
	sort.Strings(out.RectificationArchiveIDs)
	sort.Strings(out.FrozenDigestAnomalyArchiveIDs)
	sort.Strings(out.ValidCertificateArchiveIDs)
	sort.Strings(out.MissingCertificateArchiveIDs)
	sort.Strings(out.InconsistentCertificateArchiveIDs)
	return out, nil
}

type FindingQuery struct{ Severity, RuleCode, SubjectType, Decision string }
type FindingSummary struct {
	Run       *domain.CheckRun  `json:"run"`
	Counts    map[string]int    `json:"counts"`
	Findings  []*domain.Finding `json:"findings"`
	Locations map[string]any    `json:"locations,omitempty"`
}

func (s *Service) Findings(id, runID string, q FindingQuery) (FindingSummary, error) {
	a, e := s.Store.Get(id)
	if e != nil {
		return FindingSummary{}, e
	}
	run := a.CheckRuns[runID]
	if run == nil {
		return FindingSummary{}, domain.ErrNotFound
	}
	if run.RevisionID == "" {
		return FindingSummary{}, nil
	}
	allFindings := findingsForRun(a, run)
	if !runConsistent(a, run, allFindings) {
		return FindingSummary{}, &domain.BusinessError{Cause: domain.ErrDigestMismatch, Code: "check_run_digest_mismatch", Message: "质检运行摘要不一致"}
	}
	run.Consistent = true
	counts := map[string]int{}
	out := []*domain.Finding{}
	for _, fid := range run.FindingIDs {
		f := a.Findings[fid]
		if f == nil {
			continue
		}
		counts[f.RuleCode]++
		counts["severity:"+f.Severity]++
		counts["subject:"+f.SubjectType]++
		if q.Severity != "" && f.Severity != q.Severity {
			continue
		}
		if q.RuleCode != "" && f.RuleCode != q.RuleCode {
			continue
		}
		if q.SubjectType != "" && f.SubjectType != q.SubjectType {
			continue
		}
		if q.Decision != "" && f.Decision != q.Decision {
			continue
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	locations := map[string]any{}
	rev := a.Revisions[run.RevisionID]
	if rev != nil {
		stations := map[string]domain.Station{}
		legs := map[string]domain.Leg{}
		for _, st := range rev.Stations {
			stations[st.ID] = st
		}
		for _, lg := range rev.Legs {
			legs[lg.ID] = lg
		}
		for _, f := range out {
			switch f.SubjectType {
			case "station":
				if st, ok := stations[f.SubjectID]; ok {
					locations[f.ID] = map[string]any{"subject": st}
				}
			case "leg":
				if lg, ok := legs[f.SubjectID]; ok {
					locations[f.ID] = map[string]any{"subject": lg, "from": stations[lg.From], "to": stations[lg.To]}
				}
			case "archive":
				ids := []string{}
				for _, st := range rev.Stations {
					ids = append(ids, st.ID)
				}
				sort.Strings(ids)
				locations[f.ID] = map[string]any{"component": ids}
			}
		}
	}
	return FindingSummary{run, counts, out, locations}, nil
}

type TimelinePage struct {
	Events    []domain.TimelineEvent `json:"events"`
	Total     int                    `json:"total"`
	FullTotal int                    `json:"fullTotal"`
	Anomalies []string               `json:"anomalies"`
}

func (s *Service) Timeline(id, typ, actor, from, to string, page, size int) (TimelinePage, error) {
	a, e := s.Store.Get(id)
	if e != nil {
		return TimelinePage{}, e
	}
	all := append([]domain.TimelineEvent(nil), a.Timeline...)
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].At == all[j].At {
			return i < j
		}
		return all[i].At < all[j].At
	})
	anomalies := []string{}
	prev := 0
	for _, ev := range all {
		if ev.Version > 0 && ev.Version < prev {
			anomalies = append(anomalies, "版本非单调")
		}
		if ev.Version > 0 {
			prev = ev.Version
		}
		if ev.Type == "accepted" && !hasTimeline(all, "frozen", ev.At) {
			anomalies = append(anomalies, "签发早于冻结")
		}
	}
	filtered := []domain.TimelineEvent{}
	for _, ev := range all {
		if typ != "" && ev.Type != typ {
			continue
		}
		if actor != "" && ev.Actor != actor {
			continue
		}
		if from != "" && ev.At < from {
			continue
		}
		if to != "" && ev.At > to {
			continue
		}
		filtered = append(filtered, ev)
	}
	full := len(all)
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 50
	}
	if size > 200 {
		size = 200
	}
	start := (page - 1) * size
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	return TimelinePage{Events: filtered[start:end], Total: len(filtered), FullTotal: full, Anomalies: unique(anomalies)}, nil
}
func hasTimeline(es []domain.TimelineEvent, typ, at string) bool {
	for _, e := range es {
		if e.Type == typ && e.At <= at {
			return true
		}
	}
	return false
}
func unique(in []string) []string {
	m := map[string]bool{}
	o := []string{}
	for _, x := range in {
		if !m[x] {
			m[x] = true
			o = append(o, x)
		}
	}
	return o
}

type Difference struct {
	Field  string `json:"field"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type EntityDifference struct {
	ObjectType string       `json:"objectType"`
	ObjectID   string       `json:"objectId"`
	Change     string       `json:"change"`
	Fields     []Difference `json:"fields,omitempty"`
}
type DifferenceReport struct {
	Differences                                                                            []Difference       `json:"differences"`
	Entities                                                                               []EntityDifference `json:"entities"`
	AddedStations, DeletedStations, ModifiedStations, AddedLegs, DeletedLegs, ModifiedLegs int                `json:"-"`
	AffectedStations                                                                       int                `json:"affectedStations"`
	AffectedLegs                                                                           int                `json:"affectedLegs"`
}

func (s *Service) Differences(id string) ([]Difference, error) {
	a, e := s.Store.Get(id)
	if e != nil {
		return nil, e
	}
	r := a.CurrentRevision()
	if r == nil || r.ParentRevisionID == "" {
		return []Difference{}, nil
	}
	p := a.Revisions[r.ParentRevisionID]
	if p == nil {
		return []Difference{}, nil
	}
	d := []Difference{}
	if len(r.Stations) != len(p.Stations) {
		d = append(d, Difference{"stations", len(p.Stations), len(r.Stations)})
	}
	if len(r.Legs) != len(p.Legs) {
		d = append(d, Difference{"legs", len(p.Legs), len(r.Legs)})
	}
	if r.ChangeSummary != p.ChangeSummary {
		d = append(d, Difference{"changeSummary", p.ChangeSummary, r.ChangeSummary})
	}
	return d, nil
}
func (s *Service) DifferenceReport(id string) (DifferenceReport, error) {
	a, e := s.Store.Get(id)
	if e != nil {
		return DifferenceReport{}, e
	}
	r := a.CurrentRevision()
	if r == nil || r.ParentRevisionID == "" {
		return DifferenceReport{Differences: []Difference{}, Entities: []EntityDifference{}}, nil
	}
	p := a.Revisions[r.ParentRevisionID]
	if p == nil {
		return DifferenceReport{Differences: []Difference{}, Entities: []EntityDifference{}}, nil
	}
	s.differenceMu.Lock()
	if cached, ok := s.differenceCache[id]; ok && cached.revisionID == r.ID {
		s.differenceMu.Unlock()
		return cloneDifferenceReport(cached.report), nil
	}
	s.differenceMu.Unlock()
	out := DifferenceReport{Differences: []Difference{}, Entities: []EntityDifference{}}
	pm := map[string]domain.Station{}
	rm := map[string]domain.Station{}
	for _, x := range p.Stations {
		pm[x.ID] = x
	}
	for _, x := range r.Stations {
		rm[x.ID] = x
	}
	for id, b := range pm {
		aft, ok := rm[id]
		if !ok {
			out.Entities = append(out.Entities, EntityDifference{"station", id, "deleted", nil})
			out.DeletedStations++
			continue
		}
		fields := []Difference{}
		if b.Name != aft.Name {
			fields = append(fields, Difference{"name", b.Name, aft.Name})
		}
		if b.X != aft.X {
			fields = append(fields, Difference{"x", b.X, aft.X})
		}
		if b.Y != aft.Y {
			fields = append(fields, Difference{"y", b.Y, aft.Y})
		}
		if b.Z != aft.Z {
			fields = append(fields, Difference{"z", b.Z, aft.Z})
		}
		if len(fields) > 0 {
			out.Entities = append(out.Entities, EntityDifference{"station", id, "modified", fields})
			out.ModifiedStations++
		}
	}
	for id := range rm {
		if _, ok := pm[id]; !ok {
			out.Entities = append(out.Entities, EntityDifference{"station", id, "added", nil})
			out.AddedStations++
		}
	}
	pl, rl := map[string]domain.Leg{}, map[string]domain.Leg{}
	for _, x := range p.Legs {
		pl[x.ID] = x
	}
	for _, x := range r.Legs {
		rl[x.ID] = x
	}
	for id, b := range pl {
		aft, ok := rl[id]
		if !ok {
			out.Entities = append(out.Entities, EntityDifference{"leg", id, "deleted", nil})
			out.DeletedLegs++
			continue
		}
		fields := []Difference{}
		if b.From != aft.From {
			fields = append(fields, Difference{"from", b.From, aft.From})
		}
		if b.To != aft.To {
			fields = append(fields, Difference{"to", b.To, aft.To})
		}
		if b.Distance != aft.Distance {
			fields = append(fields, Difference{"distance", b.Distance, aft.Distance})
		}
		if b.Azimuth != aft.Azimuth {
			fields = append(fields, Difference{"azimuth", b.Azimuth, aft.Azimuth})
		}
		if b.Inclination != aft.Inclination {
			fields = append(fields, Difference{"inclination", b.Inclination, aft.Inclination})
		}
		if len(fields) > 0 {
			out.Entities = append(out.Entities, EntityDifference{"leg", id, "modified", fields})
			out.ModifiedLegs++
		}
	}
	for id := range rl {
		if _, ok := pl[id]; !ok {
			out.Entities = append(out.Entities, EntityDifference{"leg", id, "added", nil})
			out.AddedLegs++
		}
	}
	sort.Slice(out.Entities, func(i, j int) bool {
		if out.Entities[i].ObjectType == out.Entities[j].ObjectType {
			return out.Entities[i].ObjectID < out.Entities[j].ObjectID
		}
		return out.Entities[i].ObjectType < out.Entities[j].ObjectType
	})
	out.AffectedStations = out.AddedStations + out.DeletedStations + out.ModifiedStations
	out.AffectedLegs = out.AddedLegs + out.DeletedLegs + out.ModifiedLegs
	s.differenceMu.Lock()
	s.differenceCache[id] = differenceCacheEntry{revisionID: r.ID, report: cloneDifferenceReport(out)}
	s.differenceMu.Unlock()
	return out, nil
}

// cloneDifferenceReport returns a deep copy of the report whose slice headers
// and backing arrays are independent from the source. Callers may freely
// mutate the returned Differences, Entities and per-entity Fields without
// affecting the canonical report stored in the difference cache. Before/After
// values are immutable primitives (string/int/float64), so a shallow copy of
// each Difference is sufficient.
func cloneDifferenceReport(r DifferenceReport) DifferenceReport {
	out := r
	if r.Differences != nil {
		out.Differences = append([]Difference(nil), r.Differences...)
	}
	if r.Entities == nil {
		return out
	}
	entities := make([]EntityDifference, len(r.Entities))
	for i, e := range r.Entities {
		entities[i] = e
		if e.Fields != nil {
			entities[i].Fields = append([]Difference(nil), e.Fields...)
		}
	}
	out.Entities = entities
	return out
}
