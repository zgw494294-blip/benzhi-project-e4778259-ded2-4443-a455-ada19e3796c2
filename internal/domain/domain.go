package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Status string

const (
	StatusDraft        Status = "draft"
	StatusPendingCheck Status = "pending_check"
	StatusReviewing    Status = "reviewing"
	StatusRework       Status = "rework"
	StatusFreezable    Status = "freezable"
	StatusFrozen       Status = "frozen"
	StatusAccepted     Status = "accepted"
)

type Station struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Z    float64 `json:"z"`
}
type Leg struct {
	ID          string  `json:"id"`
	From        string  `json:"from"`
	To          string  `json:"to"`
	Distance    float64 `json:"distance"`
	Azimuth     float64 `json:"azimuth"`
	Inclination float64 `json:"inclination"`
}
type Revision struct {
	ID               string    `json:"id"`
	ArchiveID        string    `json:"archiveId"`
	RevisionNumber   int       `json:"revisionNumber"`
	ParentRevisionID string    `json:"parentRevisionId,omitempty"`
	Stations         []Station `json:"stations"`
	Legs             []Leg     `json:"legs"`
	ChangeSummary    string    `json:"changeSummary,omitempty"`
	SubmittedBy      string    `json:"submittedBy"`
	SubmittedAt      string    `json:"submittedAt"`
	ContentHash      string    `json:"contentHash"`
	IdempotencyKey   string    `json:"idempotencyKey,omitempty"`
	RequestDigest    string    `json:"requestDigest,omitempty"`
}
type Finding struct {
	ID              string `json:"id"`
	CheckRunID      string `json:"checkRunId"`
	RuleCode        string `json:"ruleCode"`
	Severity        string `json:"severity"`
	SubjectType     string `json:"subjectType"`
	SubjectID       string `json:"subjectId"`
	Message         string `json:"message"`
	Decision        string `json:"decision,omitempty"`
	DecisionReason  string `json:"decisionReason,omitempty"`
	ReviewedBy      string `json:"reviewedBy,omitempty"`
	ReviewedAt      string `json:"reviewedAt,omitempty"`
	SupersededBy    string `json:"supersededBy,omitempty"`
	TraceStatus     string `json:"traceStatus,omitempty"`
	ReviewerRole    string `json:"reviewerRole,omitempty"`
	SecondReviewer  string `json:"secondReviewer,omitempty"`
	Rectification   string `json:"rectification,omitempty"`
	RelatedObject   string `json:"relatedObject,omitempty"`
	SourceFindingID string `json:"sourceFindingId,omitempty"`
}
type CheckRun struct {
	ID             string   `json:"id"`
	ArchiveID      string   `json:"archiveId"`
	RevisionID     string   `json:"revisionId"`
	RuleSetVersion string   `json:"ruleSetVersion"`
	StartedAt      string   `json:"startedAt"`
	CompletedAt    string   `json:"completedAt"`
	Result         string   `json:"result"`
	SummaryHash    string   `json:"summaryHash"`
	InputHash      string   `json:"inputHash,omitempty"`
	FindingsHash   string   `json:"findingsHash,omitempty"`
	Consistent     bool     `json:"consistent,omitempty"`
	FindingIDs     []string `json:"findingIds"`
}
type Manifest struct {
	ManifestID   string    `json:"manifestId"`
	ArchiveID    string    `json:"archiveId"`
	RevisionID   string    `json:"revisionId"`
	StationCount int       `json:"stationCount"`
	LegCount     int       `json:"legCount"`
	ContentHash  string    `json:"contentHash"`
	FrozenBy     string    `json:"frozenBy"`
	FrozenAt     string    `json:"frozenAt"`
	EntityHash   string    `json:"entityHash,omitempty"`
	ManifestHash string    `json:"manifestHash,omitempty"`
	Stations     []Station `json:"stations,omitempty"`
	Legs         []Leg     `json:"legs,omitempty"`
}
type Certificate struct {
	CertificateID   string `json:"certificateId"`
	ArchiveID       string `json:"archiveId"`
	ManifestID      string `json:"manifestId"`
	ContentHash     string `json:"contentHash"`
	IssuedBy        string `json:"issuedBy"`
	IssuedAt        string `json:"issuedAt"`
	CertificateHash string `json:"certificateHash"`
	IdempotencyKey  string `json:"idempotencyKey,omitempty"`
	RequestDigest   string `json:"requestDigest,omitempty"`
}
type TimelineEvent struct {
	Type          string `json:"type"`
	At            string `json:"at"`
	Actor         string `json:"actor"`
	Detail        string `json:"detail"`
	Version       int    `json:"version,omitempty"`
	RevisionID    string `json:"revisionId,omitempty"`
	CheckRunID    string `json:"checkRunId,omitempty"`
	ManifestID    string `json:"manifestId,omitempty"`
	CertificateID string `json:"certificateId,omitempty"`
}

type RowError struct {
	ObjectType string `json:"objectType"`
	Row        int    `json:"row"`
	Field      string `json:"field"`
	Reason     string `json:"reason"`
}
type RevisionValidation struct {
	Valid  bool       `json:"valid"`
	Errors []RowError `json:"errors"`
}

type Archive struct {
	ID                   string               `json:"id"`
	ArchiveCode          string               `json:"archiveCode"`
	CaveName             string               `json:"caveName"`
	SurveyDate           string               `json:"surveyDate"`
	CoordinateDatum      string               `json:"coordinateDatum"`
	Status               Status               `json:"status"`
	CurrentRevisionID    string               `json:"currentRevisionId,omitempty"`
	Version              int                  `json:"version"`
	CreatedAt            string               `json:"createdAt"`
	UpdatedAt            string               `json:"updatedAt"`
	Revisions            map[string]*Revision `json:"revisions"`
	CheckRuns            map[string]*CheckRun `json:"checkRuns"`
	Findings             map[string]*Finding  `json:"findings"`
	Manifest             *Manifest            `json:"manifest,omitempty"`
	Certificate          *Certificate         `json:"certificate,omitempty"`
	Timeline             []TimelineEvent      `json:"timeline"`
	CreateIdempotencyKey string               `json:"createIdempotencyKey,omitempty"`
	CreateRequestDigest  string               `json:"createRequestDigest,omitempty"`
}

var (
	ErrNotFound            = errors.New("未找到资源")
	ErrConflict            = errors.New("版本冲突")
	ErrInvalid             = errors.New("输入无效")
	ErrForbidden           = errors.New("当前状态不允许此操作")
	ErrIdempotencyConflict = errors.New("幂等参数冲突")
	ErrArchiveCodeConflict = errors.New("归档编号冲突")
	ErrDuplicateContent    = errors.New("修订内容重复")
	ErrDigestMismatch      = errors.New("摘要一致性校验失败")
	ErrUnsupportedRuleSet  = errors.New("不支持的规则集版本")
)

type BusinessError struct {
	Cause              error
	Code               string
	Message            string
	ExistingArchiveID  string
	ExistingRevisionID string
	FindingIDs         []string
	Field              string
}

func (e *BusinessError) Error() string { return e.Message }
func (e *BusinessError) Unwrap() error { return e.Cause }

func NewArchive(code, cave, date, datum string) (*Archive, error) {
	code, cave, date, datum = strings.TrimSpace(code), strings.TrimSpace(cave), strings.TrimSpace(date), strings.TrimSpace(datum)
	if strings.TrimSpace(code) == "" || strings.TrimSpace(cave) == "" || strings.TrimSpace(date) == "" || strings.TrimSpace(datum) == "" {
		return nil, fmt.Errorf("%w: 归档包元数据不完整", ErrInvalid)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := fmt.Sprintf("arc-%d", time.Now().UnixNano())
	return &Archive{ID: id, ArchiveCode: code, CaveName: cave, SurveyDate: date, CoordinateDatum: datum, Status: StatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now, Revisions: map[string]*Revision{}, CheckRuns: map[string]*CheckRun{}, Findings: map[string]*Finding{}, Timeline: []TimelineEvent{{Type: "created", At: now, Actor: "system", Detail: "创建归档包", Version: 1}}}, nil
}

func (a *Archive) touch(actor, typ, detail string) {
	a.Version++
	a.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	a.Timeline = append(a.Timeline, TimelineEvent{Type: typ, At: a.UpdatedAt, Actor: actor, Detail: detail, Version: a.Version, RevisionID: a.CurrentRevisionID, ManifestID: func() string {
		if a.Manifest != nil {
			return a.Manifest.ManifestID
		}
		return ""
	}(), CertificateID: func() string {
		if a.Certificate != nil {
			return a.Certificate.CertificateID
		}
		return ""
	}()})
}
func (a *Archive) ensureVersion(v int) error {
	if v > 0 && a.Version != v {
		return fmt.Errorf("%w: 当前版本为%d", ErrConflict, a.Version)
	}
	return nil
}
func (a *Archive) AddRevision(r *Revision, expected int) error {
	if err := a.ensureVersion(expected); err != nil {
		return err
	}
	if !a.Status.AllowsRevision() {
		return fmt.Errorf("%w: 只能在草稿或待返修状态登记修订", ErrForbidden)
	}
	if strings.TrimSpace(r.SubmittedBy) == "" {
		return fmt.Errorf("%w: 提交者不能为空", ErrInvalid)
	}
	if r.ParentRevisionID != "" {
		parent := a.Revisions[r.ParentRevisionID]
		if parent == nil || parent.ArchiveID != a.ID {
			return fmt.Errorf("%w: 父修订不属于当前归档", ErrInvalid)
		}
	}
	if a.Status == StatusRework && r.ParentRevisionID != a.CurrentRevisionID {
		return fmt.Errorf("%w: 返修修订必须引用当前被退回修订", ErrInvalid)
	}
	vr := ValidateRevision(r)
	if !vr.Valid {
		return fmt.Errorf("%w: 批量数据校验失败: %s", ErrInvalid, vr.Errors[0].Reason)
	}
	normalizeRevision(r)
	r.ID = fmt.Sprintf("rev-%d", time.Now().UnixNano())
	r.ArchiveID = a.ID
	r.RevisionNumber = len(a.Revisions) + 1
	r.SubmittedBy = strings.TrimSpace(r.SubmittedBy)
	r.ChangeSummary = strings.TrimSpace(r.ChangeSummary)
	r.SubmittedAt = time.Now().UTC().Format(time.RFC3339Nano)
	r.ContentHash = RevisionContentHash(r)
	a.Revisions[r.ID] = r
	a.CurrentRevisionID = r.ID
	a.Status = StatusDraft
	a.touch(r.SubmittedBy, "revision_created", fmt.Sprintf("登记修订%d", r.RevisionNumber))
	return nil
}

func normalizeRevision(r *Revision) {
	for i := range r.Stations {
		r.Stations[i].ID = strings.TrimSpace(r.Stations[i].ID)
		r.Stations[i].Name = strings.TrimSpace(r.Stations[i].Name)
	}
	for i := range r.Legs {
		r.Legs[i].ID = strings.TrimSpace(r.Legs[i].ID)
		r.Legs[i].From = strings.TrimSpace(r.Legs[i].From)
		r.Legs[i].To = strings.TrimSpace(r.Legs[i].To)
	}
}

func ValidateRevision(r *Revision) RevisionValidation {
	vr := RevisionValidation{Valid: true}
	if r == nil {
		return RevisionValidation{Valid: false, Errors: []RowError{{ObjectType: "revision", Row: 0, Field: "revision", Reason: "修订不能为空"}}}
	}
	if len(r.Stations)+len(r.Legs) > 2000 {
		return RevisionValidation{Valid: false, Errors: []RowError{{ObjectType: "revision", Row: 0, Field: "rows", Reason: "批量行数不能超过2000"}}}
	}
	if len(r.Stations) == 0 {
		vr.Valid = false
		vr.Errors = append(vr.Errors, RowError{"station", 0, "stations", "至少需要一个测站"})
	}
	ids, names := map[string]int{}, map[string]int{}
	for i, s := range r.Stations {
		id, n := strings.TrimSpace(s.ID), strings.TrimSpace(s.Name)
		if id == "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"station", i + 1, "id", "测站编号不能为空"})
		}
		if n == "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"station", i + 1, "name", "测站名称不能为空"})
		}
		if p, ok := ids[id]; ok && id != "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"station", i + 1, "id", fmt.Sprintf("重复测站编号，首次出现在第%d行", p)})
		} else if id != "" {
			ids[id] = i + 1
		}
		if p, ok := names[n]; ok && n != "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"station", i + 1, "name", fmt.Sprintf("重复测站名称，首次出现在第%d行", p)})
		} else if n != "" {
			names[n] = i + 1
		}
	}
	legIDs := map[string]int{}
	pairs := map[string]int{}
	for i, l := range r.Legs {
		id, from, to := strings.TrimSpace(l.ID), strings.TrimSpace(l.From), strings.TrimSpace(l.To)
		if id == "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "id", "测段编号不能为空"})
		}
		if p, ok := legIDs[id]; ok && id != "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "id", fmt.Sprintf("重复测段编号，首次出现在第%d行", p)})
		} else if id != "" {
			legIDs[id] = i + 1
		}
		if from == "" || to == "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "from/to", "测段端点不能为空"})
		}
		if from == to && from != "" {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "to", "测段不能自连接"})
		}
		if from != "" && to != "" {
			if !hasID(ids, from) {
				vr.Valid = false
				vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "from", "测段引用不存在的测站"})
			}
			if !hasID(ids, to) {
				vr.Valid = false
				vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "to", "测段引用不存在的测站"})
			}
			key := from + "\x00" + to
			rev := to + "\x00" + from
			if p, ok := pairs[key]; ok {
				vr.Valid = false
				vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "from/to", fmt.Sprintf("重复测段，首次出现在第%d行", p)})
			}
			if p, ok := pairs[rev]; ok {
				vr.Valid = false
				vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "from/to", fmt.Sprintf("重复测段，首次出现在第%d行", p)})
			}
			pairs[key] = i + 1
		}
		if l.Distance <= 0 || l.Distance > 100000 {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "distance", "距离必须大于0且不超过100000"})
		}
		if l.Azimuth < 0 || l.Azimuth >= 360 {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "azimuth", "方位角必须在0到360之间"})
		}
		if l.Inclination < -90 || l.Inclination > 90 {
			vr.Valid = false
			vr.Errors = append(vr.Errors, RowError{"leg", i + 1, "inclination", "倾角必须在-90到90之间"})
		}
	}
	return vr
}
func hasID(m map[string]int, id string) bool { _, ok := m[id]; return ok }

type DecisionInput struct {
	FindingID      string `json:"findingId"`
	Decision       string `json:"decision"`
	Reason         string `json:"reason"`
	Reviewer       string `json:"reviewer"`
	ReviewerRole   string `json:"reviewerRole"`
	SecondReviewer string `json:"secondReviewer"`
	Rectification  string `json:"rectification"`
	RelatedObject  string `json:"relatedObject"`
}

func (a *Archive) BatchDecide(items []DecisionInput, expected int) error {
	if err := a.ensureVersion(expected); err != nil {
		return err
	}
	if a.Status != StatusReviewing {
		return fmt.Errorf("%w: 当前不在校审中", ErrForbidden)
	}
	if len(items) == 0 {
		return fmt.Errorf("%w: 至少选择一条发现", ErrInvalid)
	}
	for _, in := range items {
		f := a.Findings[in.FindingID]
		if f == nil || f.SupersededBy != "" {
			return fmt.Errorf("%w: 发现%s不是当前活动发现", ErrInvalid, in.FindingID)
		}
		if in.Decision != "confirm" && in.Decision != "waive" && in.Decision != "rectify" {
			return fmt.Errorf("%w: 裁决必须为confirm、waive或rectify", ErrInvalid)
		}
		if strings.TrimSpace(in.Reason) == "" || strings.TrimSpace(in.Reviewer) == "" {
			return fmt.Errorf("%w: 裁决理由和校审人不能为空", ErrInvalid)
		}
		if f.Severity == "error" && in.Decision == "waive" && (strings.TrimSpace(in.SecondReviewer) == "" || strings.EqualFold(strings.TrimSpace(in.SecondReviewer), strings.TrimSpace(in.Reviewer))) {
			return &BusinessError{Cause: ErrInvalid, Code: "second_reviewer_required", Message: "error发现豁免需要不同的二次复核人", FindingIDs: []string{in.FindingID}}
		}
		if strings.TrimSpace(in.SecondReviewer) != "" && strings.EqualFold(strings.TrimSpace(in.SecondReviewer), strings.TrimSpace(in.Reviewer)) {
			return &BusinessError{Cause: ErrInvalid, Code: "reviewer_must_differ", Message: "二次复核人必须与主校审人不同", FindingIDs: []string{in.FindingID}}
		}
		if in.Decision == "rectify" && (strings.TrimSpace(in.Rectification) == "" || strings.TrimSpace(in.RelatedObject) == "") {
			return &BusinessError{Cause: ErrInvalid, Code: "rectification_required", Message: "整改裁决必须填写整改说明和关联对象", FindingIDs: []string{in.FindingID}}
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, in := range items {
		f := a.Findings[in.FindingID]
		f.Decision = in.Decision
		f.DecisionReason = in.Reason
		f.ReviewedBy = in.Reviewer
		f.ReviewedAt = now
		f.ReviewerRole = strings.TrimSpace(in.ReviewerRole)
		f.SecondReviewer = strings.TrimSpace(in.SecondReviewer)
		f.Rectification = strings.TrimSpace(in.Rectification)
		f.RelatedObject = strings.TrimSpace(in.RelatedObject)
	}
	a.touch(items[0].Reviewer, "findings_decided", fmt.Sprintf("批量裁决%d条发现", len(items)))
	return nil
}

type ReviewGate struct {
	Unresolved             int      `json:"unresolved"`
	Rectify                int      `json:"rectify"`
	Waived                 int      `json:"waived"`
	ExpectedStatus         Status   `json:"expectedStatus"`
	ActiveRunID            string   `json:"activeRunId"`
	HighRiskPending        int      `json:"highRiskPending"`
	UnresolvedFindingIDs   []string `json:"unresolvedFindingIds"`
	HighRiskFindingIDs     []string `json:"highRiskFindingIds"`
	RectifyFindingIDs      []string `json:"rectifyFindingIds"`
	TraceBlockedFindingIDs []string `json:"traceBlockedFindingIds"`
}

func (a *Archive) ReviewPreview() ReviewGate {
	g := ReviewGate{}
	for _, run := range a.CheckRuns {
		if run.RevisionID == a.CurrentRevisionID && (g.ActiveRunID == "" || run.CompletedAt > a.CheckRuns[g.ActiveRunID].CompletedAt || (run.CompletedAt == a.CheckRuns[g.ActiveRunID].CompletedAt && run.ID > g.ActiveRunID)) {
			g.ActiveRunID = run.ID
		}
	}
	for _, f := range a.Findings {
		if f.CheckRunID != g.ActiveRunID || f.SupersededBy != "" {
			continue
		}
		switch f.Decision {
		case "rectify":
			g.Rectify++
			g.RectifyFindingIDs = append(g.RectifyFindingIDs, f.ID)
		case "waive":
			g.Waived++
		case "":
			g.Unresolved++
			g.UnresolvedFindingIDs = append(g.UnresolvedFindingIDs, f.ID)
		}
		if f.Severity == "error" && f.Decision == "waive" && strings.TrimSpace(f.SecondReviewer) == "" {
			g.HighRiskPending++
			g.HighRiskFindingIDs = append(g.HighRiskFindingIDs, f.ID)
		}
	}
	for _, f := range a.Findings {
		if f.Decision == "rectify" && f.CheckRunID != g.ActiveRunID && f.TraceStatus != "resolved" {
			g.TraceBlockedFindingIDs = append(g.TraceBlockedFindingIDs, f.ID)
		}
	}
	sort.Strings(g.UnresolvedFindingIDs)
	sort.Strings(g.HighRiskFindingIDs)
	sort.Strings(g.RectifyFindingIDs)
	sort.Strings(g.TraceBlockedFindingIDs)
	if g.Rectify > 0 {
		g.ExpectedStatus = StatusRework
	} else {
		g.ExpectedStatus = StatusFreezable
	}
	return g
}
func (a *Archive) Submit(expected int) error {
	if err := a.ensureVersion(expected); err != nil {
		return err
	}
	if a.Status != StatusDraft || a.CurrentRevisionID == "" {
		return fmt.Errorf("%w: 当前无可提交修订", ErrForbidden)
	}
	a.Status = StatusPendingCheck
	a.touch("surveyor", "submitted", "提交待质检")
	return nil
}
func (a *Archive) StartReview(run *CheckRun, findings []*Finding, expected int) error {
	if err := a.ensureVersion(expected); err != nil {
		return err
	}
	if a.Status != StatusPendingCheck {
		return fmt.Errorf("%w: 仅待质检归档可检查", ErrForbidden)
	}
	for _, old := range a.Findings {
		if old.SupersededBy != "" || old.CheckRunID == run.ID {
			continue
		}
		matched := ""
		for _, nf := range findings {
			if nf.RuleCode == old.RuleCode && nf.SubjectType == old.SubjectType && nf.SubjectID == old.SubjectID {
				matched = nf.ID
				break
			}
		}
		if matched != "" {
			old.SupersededBy = matched
			old.TraceStatus = "still_exists"
			for _, nf := range findings {
				if nf.ID == matched {
					nf.SourceFindingID = old.ID
				}
			}
		} else if old.Decision == "rectify" {
			old.TraceStatus = "resolved"
			old.SupersededBy = run.ID
		} else {
			old.SupersededBy = run.ID
		}
	}
	a.CheckRuns[run.ID] = run
	for _, f := range findings {
		a.Findings[f.ID] = f
	}
	a.Status = StatusReviewing
	a.touch("checker", "check_run", fmt.Sprintf("生成%d条发现", len(findings)))
	if len(a.Timeline) > 0 {
		a.Timeline[len(a.Timeline)-1].CheckRunID = run.ID
	}
	return nil
}
func (a *Archive) Decide(id, decision, reason, reviewer string, expected int) error {
	f := a.Findings[id]
	related := ""
	if f != nil {
		related = f.SubjectType + ":" + f.SubjectID
	}
	return a.BatchDecide([]DecisionInput{{FindingID: id, Decision: decision, Reason: reason, Reviewer: reviewer, Rectification: reason, RelatedObject: related}}, expected)
}
func (a *Archive) FinalizeReview(expected int) error {
	if err := a.ensureVersion(expected); err != nil {
		return err
	}
	if a.Status != StatusReviewing {
		return fmt.Errorf("%w: 当前不在校审中", ErrForbidden)
	}
	g := a.ReviewPreview()
	blocked := append(append([]string{}, g.UnresolvedFindingIDs...), g.HighRiskFindingIDs...)
	if g.Rectify == 0 {
		blocked = append(blocked, g.TraceBlockedFindingIDs...)
	}
	if len(blocked) > 0 {
		sort.Strings(blocked)
		return &BusinessError{Cause: ErrInvalid, Code: "review_gate_blocked", Message: "校审门禁未通过", FindingIDs: blocked}
	}
	if g.Rectify > 0 {
		a.Status = StatusRework
	} else {
		a.Status = StatusFreezable
	}
	a.touch("reviewer", "review_completed", "完成发现裁决")
	if len(a.Timeline) > 0 {
		a.Timeline[len(a.Timeline)-1].CheckRunID = g.ActiveRunID
	}
	return nil
}
func (a *Archive) Freeze(expected int, by string) (*Manifest, error) {
	if err := a.ensureVersion(expected); err != nil {
		return nil, err
	}
	if a.Status != StatusFreezable {
		return nil, fmt.Errorf("%w: 归档尚未满足冻结门禁", ErrForbidden)
	}
	r := a.Revisions[a.CurrentRevisionID]
	if strings.TrimSpace(by) == "" {
		return nil, fmt.Errorf("%w: 冻结人不能为空", ErrInvalid)
	}
	stations, legs := StableEntities(r)
	entityHash := EntitySnapshotHash(stations, legs)
	m := &Manifest{ManifestID: fmt.Sprintf("man-%d", time.Now().UnixNano()), ArchiveID: a.ID, RevisionID: r.ID, StationCount: len(stations), LegCount: len(legs), ContentHash: r.ContentHash, EntityHash: entityHash, Stations: stations, Legs: legs, FrozenBy: strings.TrimSpace(by), FrozenAt: time.Now().UTC().Format(time.RFC3339Nano)}
	m.ManifestHash = ManifestDigest(m)
	a.Manifest = m
	a.Status = StatusFrozen
	a.touch(by, "frozen", "冻结成果清单")
	return m, nil
}
func (a *Archive) FreezeWithPreview(expected, previewVersion int, previewHash, by string) (*Manifest, error) {
	if previewVersion <= 0 || previewVersion != a.Version {
		return nil, fmt.Errorf("%w: 冻结预览版本已过期", ErrConflict)
	}
	p, err := a.PreviewFreeze()
	if err != nil {
		return nil, err
	}
	if previewHash == "" || p.PreviewHash != previewHash {
		return nil, fmt.Errorf("%w: 冻结预览摘要已变化", ErrConflict)
	}
	return a.Freeze(expected, by)
}

type FreezePreview struct {
	ArchiveID      string    `json:"archiveId"`
	RevisionID     string    `json:"revisionId"`
	Version        int       `json:"version"`
	StationCount   int       `json:"stationCount"`
	LegCount       int       `json:"legCount"`
	ContentHash    string    `json:"contentHash"`
	RecomputedHash string    `json:"recomputedHash"`
	Consistent     bool      `json:"consistent"`
	ReadOnly       bool      `json:"readOnly"`
	PreviewVersion int       `json:"previewVersion"`
	PreviewHash    string    `json:"previewHash"`
	Stations       []Station `json:"stations"`
	Legs           []Leg     `json:"legs"`
	EntityHash     string    `json:"entityHash"`
}

func (a *Archive) PreviewFreeze() (*FreezePreview, error) {
	if a.Manifest != nil {
		recomputed := ""
		if r := a.Revisions[a.Manifest.RevisionID]; r != nil {
			recomputed = RevisionContentHash(r)
		}
		p := &FreezePreview{ArchiveID: a.ID, RevisionID: a.Manifest.RevisionID, Version: a.Version, PreviewVersion: a.Version, StationCount: a.Manifest.StationCount, LegCount: a.Manifest.LegCount, ContentHash: a.Manifest.ContentHash, RecomputedHash: recomputed, Consistent: recomputed != "" && recomputed == a.Manifest.ContentHash && VerifyManifest(a.Manifest), ReadOnly: true, Stations: append([]Station(nil), a.Manifest.Stations...), Legs: append([]Leg(nil), a.Manifest.Legs...), EntityHash: a.Manifest.EntityHash}
		p.PreviewHash = FreezePreviewDigest(p)
		if r := a.Revisions[a.Manifest.RevisionID]; r != nil {
			p.Consistent = p.Consistent && ManifestMatchesRevision(a.Manifest, r)
		}
		return p, nil
	}
	if a.Status != StatusFreezable {
		return nil, fmt.Errorf("%w: 归档尚未满足冻结门禁", ErrForbidden)
	}
	r := a.CurrentRevision()
	if r == nil {
		return nil, fmt.Errorf("%w: 无可冻结修订", ErrInvalid)
	}
	h := RevisionContentHash(r)
	stations, legs := StableEntities(r)
	p := &FreezePreview{ArchiveID: a.ID, RevisionID: r.ID, Version: a.Version, PreviewVersion: a.Version, StationCount: len(stations), LegCount: len(legs), ContentHash: r.ContentHash, RecomputedHash: h, Consistent: h == r.ContentHash, Stations: stations, Legs: legs, EntityHash: EntitySnapshotHash(stations, legs)}
	p.PreviewHash = FreezePreviewDigest(p)
	return p, nil
}
func (a *Archive) Issue(by string, expected int) (*Certificate, error) {
	if err := a.ensureVersion(expected); err != nil {
		return nil, err
	}
	if a.Status != StatusFrozen || a.Manifest == nil {
		return nil, fmt.Errorf("%w: 仅已冻结归档可签发", ErrForbidden)
	}
	if strings.TrimSpace(by) == "" {
		return nil, fmt.Errorf("%w: 签发人不能为空", ErrInvalid)
	}
	if !ManifestMatchesRevision(a.Manifest, a.Revisions[a.Manifest.RevisionID]) {
		return nil, fmt.Errorf("%w: 冻结清单摘要不一致", ErrDigestMismatch)
	}
	c := &Certificate{CertificateID: fmt.Sprintf("cert-%d", time.Now().UnixNano()), ArchiveID: a.ID, ManifestID: a.Manifest.ManifestID, ContentHash: a.Manifest.ContentHash, IssuedBy: by, IssuedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	c.CertificateHash = Hash(c)
	a.Certificate = c
	a.Status = StatusAccepted
	a.touch(by, "accepted", "签发归档验收凭据")
	return c, nil
}

type CertificateVerification struct {
	Result              string          `json:"result"`
	CertificateID       string          `json:"certificateId,omitempty"`
	ArchiveID           string          `json:"archiveId,omitempty"`
	RevisionHashOK      bool            `json:"revisionHashOk"`
	ManifestReferenceOK bool            `json:"manifestReferenceOk"`
	CertificateHashOK   bool            `json:"certificateHashOk"`
	Problems            []string        `json:"problems,omitempty"`
	TimelineEvents      []TimelineEvent `json:"timelineEvents"`
}

func (a *Archive) VerifyCertificate() CertificateVerification {
	if a.Certificate == nil {
		return CertificateVerification{Result: "not_found"}
	}
	c := a.Certificate
	v := CertificateVerification{Result: "valid", CertificateID: c.CertificateID, ArchiveID: a.ID}
	for _, event := range a.Timeline {
		if event.CertificateID == c.CertificateID || event.Type == "frozen" {
			v.TimelineEvents = append(v.TimelineEvents, event)
		}
	}
	m := a.Manifest
	if m == nil || m.ManifestID != c.ManifestID || m.ArchiveID != a.ID || !VerifyManifest(m) {
		v.ManifestReferenceOK = false
		v.Problems = append(v.Problems, "清单引用不一致")
	} else {
		v.ManifestReferenceOK = true
		r := a.Revisions[m.RevisionID]
		if !ManifestMatchesRevision(m, r) || c.ContentHash != m.ContentHash {
			v.RevisionHashOK = false
			v.Problems = append(v.Problems, "修订摘要不一致")
		} else {
			v.RevisionHashOK = true
		}
	}
	cp := *c
	cp.CertificateHash = ""
	v.CertificateHashOK = Hash(cp) == c.CertificateHash
	if !v.CertificateHashOK {
		v.Problems = append(v.Problems, "凭据哈希校验失败")
	}
	if len(v.Problems) > 0 {
		v.Result = "invalid"
	}
	return v
}
func Hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func RevisionContentHash(r *Revision) string {
	if r == nil {
		return ""
	}
	ss, ls := StableEntities(r)
	return Hash(struct {
		Stations      []Station `json:"stations"`
		Legs          []Leg     `json:"legs"`
		ChangeSummary string    `json:"changeSummary,omitempty"`
	}{ss, ls, strings.TrimSpace(r.ChangeSummary)})
}
func (a *Archive) CurrentRevision() *Revision { return a.Revisions[a.CurrentRevisionID] }
func (a *Archive) SortedFindings() []*Finding {
	out := make([]*Finding, 0, len(a.Findings))
	for _, f := range a.Findings {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
