package application

import (
	"cave-archive/internal/domain"
	"fmt"
	"sort"
	"strings"
)

type ComparisonQuery struct {
	Category string
	Severity string
	RuleCode string
}

type ComparisonSummary struct {
	Resolved    int `json:"resolved"`
	StillExists int `json:"stillExists"`
	New         int `json:"new"`
	Total       int `json:"total"`
}

type FindingComparison struct {
	Category          string             `json:"category"`
	RuleCode          string             `json:"ruleCode"`
	Severity          string             `json:"severity"`
	SubjectType       string             `json:"subjectType"`
	SubjectID         string             `json:"subjectId"`
	ParentFindingID   string             `json:"parentFindingId,omitempty"`
	CurrentFindingID  string             `json:"currentFindingId,omitempty"`
	OldDecision       string             `json:"oldDecision,omitempty"`
	OldDecisionReason string             `json:"oldDecisionReason,omitempty"`
	Rectification     string             `json:"rectification,omitempty"`
	RelatedObject     string             `json:"relatedObject,omitempty"`
	ObjectStatus      string             `json:"objectStatus"`
	Object            any                `json:"object,omitempty"`
	Differences       []EntityDifference `json:"differences"`
}

type ReworkComparisonReport struct {
	ArchiveID         string              `json:"archiveId"`
	ParentRevisionID  string              `json:"parentRevisionId"`
	CurrentRevisionID string              `json:"currentRevisionId"`
	ParentCheckRunID  string              `json:"parentCheckRunId"`
	CurrentCheckRunID string              `json:"currentCheckRunId"`
	Summary           ComparisonSummary   `json:"summary"`
	FilteredCount     int                 `json:"filteredCount"`
	Items             []FindingComparison `json:"items"`
}

func (s *Service) ReworkComparison(id, currentRunID string, query ComparisonQuery) (ReworkComparisonReport, error) {
	a, err := s.Store.Get(id)
	if err != nil {
		return ReworkComparisonReport{}, err
	}
	currentRun := a.CheckRuns[strings.TrimSpace(currentRunID)]
	if currentRun == nil {
		return ReworkComparisonReport{}, &domain.BusinessError{Cause: domain.ErrNotFound, Code: "current_check_run_missing", Message: "当前返修检查运行不存在"}
	}
	if currentRun.ArchiveID != a.ID {
		return ReworkComparisonReport{}, &domain.BusinessError{Cause: domain.ErrInvalid, Code: "cross_archive_check_run", Message: "检查运行不属于当前归档"}
	}
	currentRevision := a.Revisions[currentRun.RevisionID]
	if currentRevision == nil || currentRevision.ID != a.CurrentRevisionID || currentRevision.ParentRevisionID == "" {
		return ReworkComparisonReport{}, &domain.BusinessError{Cause: domain.ErrInvalid, Code: "invalid_rework_relation", Message: "当前检查运行未关联有效的返修父子修订"}
	}
	parentRevision := a.Revisions[currentRevision.ParentRevisionID]
	if parentRevision == nil || parentRevision.ArchiveID != a.ID {
		return ReworkComparisonReport{}, &domain.BusinessError{Cause: domain.ErrInvalid, Code: "invalid_rework_relation", Message: "返修父修订不存在或不属于当前归档"}
	}
	parentRun := latestRunForRevision(a, parentRevision.ID)
	if parentRun == nil {
		return ReworkComparisonReport{}, &domain.BusinessError{Cause: domain.ErrInvalid, Code: "baseline_check_run_missing", Message: "父修订缺少检查运行基线"}
	}
	if err := validateComparisonRun(a, parentRun); err != nil {
		return ReworkComparisonReport{}, err
	}
	if err := validateComparisonRun(a, currentRun); err != nil {
		return ReworkComparisonReport{}, err
	}

	differenceReport, err := s.DifferenceReport(id)
	if err != nil {
		return ReworkComparisonReport{}, err
	}
	differenceIndex := map[string][]EntityDifference{}
	for _, difference := range differenceReport.Entities {
		key := difference.ObjectType + "\x00" + difference.ObjectID
		differenceIndex[key] = append(differenceIndex[key], difference)
	}
	differenceIndex["archive\x00"+a.ID] = append([]EntityDifference(nil), differenceReport.Entities...)
	parentFindings := findingsForRun(a, parentRun)
	currentFindings := findingsForRun(a, currentRun)
	parentByKey := make(map[string]*domain.Finding, len(parentFindings))
	currentByKey := make(map[string]*domain.Finding, len(currentFindings))
	for _, finding := range parentFindings {
		parentByKey[findingComparisonKey(finding)] = finding
	}
	for _, finding := range currentFindings {
		currentByKey[findingComparisonKey(finding)] = finding
	}

	report := ReworkComparisonReport{
		ArchiveID:         a.ID,
		ParentRevisionID:  parentRevision.ID,
		CurrentRevisionID: currentRevision.ID,
		ParentCheckRunID:  parentRun.ID,
		CurrentCheckRunID: currentRun.ID,
		Items:             []FindingComparison{},
	}
	all := make([]FindingComparison, 0, len(parentFindings)+len(currentFindings))
	for key, parentFinding := range parentByKey {
		currentFinding := currentByKey[key]
		category := "resolved"
		objectRevision := parentRevision
		if currentFinding != nil {
			category = "still_exists"
			objectRevision = currentRevision
		}
		item := comparisonItem(category, parentFinding, currentFinding, objectRevision, differenceIndex)
		all = append(all, item)
	}
	for key, currentFinding := range currentByKey {
		if parentByKey[key] != nil {
			continue
		}
		all = append(all, comparisonItem("new", nil, currentFinding, currentRevision, differenceIndex))
	}
	categoryOrder := map[string]int{"resolved": 0, "still_exists": 1, "new": 2}
	sort.Slice(all, func(i, j int) bool {
		if categoryOrder[all[i].Category] != categoryOrder[all[j].Category] {
			return categoryOrder[all[i].Category] < categoryOrder[all[j].Category]
		}
		if all[i].RuleCode != all[j].RuleCode {
			return all[i].RuleCode < all[j].RuleCode
		}
		if all[i].SubjectType != all[j].SubjectType {
			return all[i].SubjectType < all[j].SubjectType
		}
		return all[i].SubjectID < all[j].SubjectID
	})
	for _, item := range all {
		report.Summary.Total++
		switch item.Category {
		case "resolved":
			report.Summary.Resolved++
		case "still_exists":
			report.Summary.StillExists++
		case "new":
			report.Summary.New++
		}
		if query.Category != "" && item.Category != query.Category {
			continue
		}
		if query.Severity != "" && item.Severity != query.Severity {
			continue
		}
		if query.RuleCode != "" && item.RuleCode != query.RuleCode {
			continue
		}
		report.Items = append(report.Items, item)
	}
	report.FilteredCount = len(report.Items)
	return report, nil
}

func latestRunForRevision(a *domain.Archive, revisionID string) *domain.CheckRun {
	var selected *domain.CheckRun
	for _, run := range a.CheckRuns {
		if run.RevisionID != revisionID {
			continue
		}
		if selected == nil || run.CompletedAt > selected.CompletedAt || (run.CompletedAt == selected.CompletedAt && run.ID > selected.ID) {
			selected = run
		}
	}
	return selected
}

func validateComparisonRun(a *domain.Archive, run *domain.CheckRun) error {
	if run.ArchiveID != a.ID {
		return &domain.BusinessError{Cause: domain.ErrInvalid, Code: "cross_archive_check_run", Message: "检查运行不属于当前归档"}
	}
	if !runConsistent(a, run, findingsForRun(a, run)) {
		return &domain.BusinessError{Cause: domain.ErrDigestMismatch, Code: "check_run_digest_mismatch", Message: "检查运行输入摘要或发现摘要不一致"}
	}
	return nil
}

func findingComparisonKey(finding *domain.Finding) string {
	return finding.RuleCode + "\x00" + finding.SubjectType + "\x00" + finding.SubjectID
}

func comparisonItem(category string, parent, current *domain.Finding, revision *domain.Revision, differences map[string][]EntityDifference) FindingComparison {
	active := current
	if active == nil {
		active = parent
	}
	item := FindingComparison{
		Category:     category,
		RuleCode:     active.RuleCode,
		Severity:     active.Severity,
		SubjectType:  active.SubjectType,
		SubjectID:    active.SubjectID,
		ObjectStatus: "present",
		Differences:  differences[active.SubjectType+"\x00"+active.SubjectID],
	}
	if item.Differences == nil {
		item.Differences = []EntityDifference{}
	}
	if parent != nil {
		item.ParentFindingID = parent.ID
		item.OldDecision = parent.Decision
		item.OldDecisionReason = parent.DecisionReason
		item.Rectification = parent.Rectification
		item.RelatedObject = parent.RelatedObject
	}
	if current != nil {
		item.CurrentFindingID = current.ID
	}
	item.Object, item.ObjectStatus = revisionObject(revision, active.SubjectType, active.SubjectID)
	return item
}

func revisionObject(revision *domain.Revision, objectType, objectID string) (any, string) {
	if revision == nil {
		return nil, "deleted"
	}
	switch objectType {
	case "station":
		for _, station := range revision.Stations {
			if station.ID == objectID {
				return station, "present"
			}
		}
	case "leg":
		for _, leg := range revision.Legs {
			if leg.ID == objectID {
				return leg, "present"
			}
		}
	case "archive":
		return map[string]any{"archiveId": objectID, "stationCount": len(revision.Stations), "legCount": len(revision.Legs)}, "present"
	}
	return nil, "deleted"
}

type CertificateBatchItem struct {
	CertificateID       string   `json:"certificateId"`
	Result              string   `json:"result"`
	ArchiveID           string   `json:"archiveId,omitempty"`
	IssuedBy            string   `json:"issuedBy,omitempty"`
	IssuedAt            string   `json:"issuedAt,omitempty"`
	CertificateHashOK   bool     `json:"certificateHashOk"`
	ManifestReferenceOK bool     `json:"manifestReferenceOk"`
	RevisionHashOK      bool     `json:"revisionHashOk"`
	Problems            []string `json:"problems"`
}

type CertificateBatchSummary struct {
	Valid    int `json:"valid"`
	Invalid  int `json:"invalid"`
	NotFound int `json:"notFound"`
	Total    int `json:"total"`
}

type CertificateBatchResult struct {
	Summary CertificateBatchSummary `json:"summary"`
	Items   []CertificateBatchItem  `json:"items"`
}

func (s *Service) VerifyCertificates(ids []string) (CertificateBatchResult, error) {
	if len(ids) == 0 {
		return CertificateBatchResult{}, &domain.BusinessError{Cause: domain.ErrInvalid, Code: "empty_certificate_ids", Message: "至少需要一个凭据编号", Field: "certificateIds"}
	}
	if len(ids) > 100 {
		return CertificateBatchResult{}, &domain.BusinessError{Cause: domain.ErrInvalid, Code: "certificate_batch_limit", Message: "凭据编号不能超过100个", Field: "certificateIds"}
	}
	normalized := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for i, raw := range ids {
		id := strings.TrimSpace(raw)
		if id == "" || !validCertificateID(id) {
			return CertificateBatchResult{}, &domain.BusinessError{Cause: domain.ErrInvalid, Code: "invalid_certificate_id", Message: fmt.Sprintf("第%d个凭据编号格式非法", i+1), Field: fmt.Sprintf("certificateIds[%d]", i)}
		}
		if !seen[id] {
			seen[id] = true
			normalized = append(normalized, id)
		}
	}
	archives := s.Store.ArchivesByCertificate(normalized)
	result := CertificateBatchResult{Items: make([]CertificateBatchItem, 0, len(normalized))}
	for _, id := range normalized {
		archive := archives[id]
		if archive == nil {
			result.Items = append(result.Items, CertificateBatchItem{CertificateID: id, Result: "not_found", Problems: []string{"凭据编号未找到"}})
			result.Summary.NotFound++
			continue
		}
		verification := archive.VerifyCertificate()
		item := CertificateBatchItem{
			CertificateID:       id,
			Result:              verification.Result,
			ArchiveID:           archive.ID,
			IssuedBy:            archive.Certificate.IssuedBy,
			IssuedAt:            archive.Certificate.IssuedAt,
			CertificateHashOK:   verification.CertificateHashOK,
			ManifestReferenceOK: verification.ManifestReferenceOK,
			RevisionHashOK:      verification.RevisionHashOK,
			Problems:            append([]string(nil), verification.Problems...),
		}
		if item.Problems == nil {
			item.Problems = []string{}
		}
		if item.Result == "valid" {
			result.Summary.Valid++
		} else {
			result.Summary.Invalid++
		}
		result.Items = append(result.Items, item)
	}
	result.Summary.Total = len(result.Items)
	return result, nil
}

func validCertificateID(id string) bool {
	if len(id) < 6 || len(id) > 133 || !strings.HasPrefix(id, "cert-") {
		return false
	}
	for _, char := range id[5:] {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' {
			continue
		}
		return false
	}
	return true
}
