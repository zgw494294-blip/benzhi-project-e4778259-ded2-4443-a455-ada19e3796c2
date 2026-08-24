package application

import (
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"errors"
	"testing"
)

func TestReworkComparisonClassificationAndReadOnly(t *testing.T) {
	st, _ := store.New("")
	service := New(st)
	archive, _ := service.Create("COMPARE-1", "复验洞", "2025-02-01", "CGCS2000", "create-compare")
	parent := &domain.Revision{
		Stations:    []domain.Station{{ID: "S1", Name: "入口"}, {ID: "S2", Name: "中段"}, {ID: "S3", Name: "旧孤站"}},
		Legs:        []domain.Leg{{ID: "L1", From: "S1", To: "S2", Distance: 8, Azimuth: 90}},
		SubmittedBy: "测绘员甲",
	}
	archive, _ = service.AddRevisionIdempotent(archive.ID, parent, archive.Version, "parent-revision")
	archive, _ = service.Submit(archive.ID, archive.Version)
	parentResult, err := service.Check(archive.ID, archive.Version)
	if err != nil || len(parentResult.Findings) != 2 {
		t.Fatalf("父修订检查结果错误: %#v, %v", parentResult, err)
	}
	items := make([]domain.DecisionInput, 0, len(parentResult.Findings))
	for _, finding := range parentResult.Findings {
		items = append(items, domain.DecisionInput{FindingID: finding.ID, Decision: "rectify", Reason: "按复验要求整改", Reviewer: "校审员甲", Rectification: "补齐连通关系", RelatedObject: finding.SubjectType + ":" + finding.SubjectID})
	}
	archive, _ = service.BatchDecide(archive.ID, items, archive.Version)
	archive, _ = service.CompleteReview(archive.ID, archive.Version)
	child := &domain.Revision{
		ParentRevisionID: parent.ID,
		Stations:         []domain.Station{{ID: "S1", Name: "入口"}, {ID: "S2", Name: "中段"}, {ID: "S4", Name: "新孤站"}},
		Legs:             []domain.Leg{{ID: "L1", From: "S1", To: "S2", Distance: 9, Azimuth: 90}},
		SubmittedBy:      "测绘员甲",
	}
	archive, _ = service.AddRevisionIdempotent(archive.ID, child, archive.Version, "child-revision")
	archive, _ = service.Submit(archive.ID, archive.Version)
	currentResult, err := service.Check(archive.ID, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	version, timeline := archive.Version, len(archive.Timeline)
	report, err := service.ReworkComparison(archive.ID, currentResult.Run.ID, ComparisonQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Resolved != 1 || report.Summary.StillExists != 1 || report.Summary.New != 1 || len(report.Items) != 3 {
		t.Fatalf("对照分类错误: %#v", report)
	}
	if report.Items[0].Category != "resolved" || report.Items[0].Rectification != "补齐连通关系" {
		t.Fatalf("稳定排序或旧整改信息错误: %#v", report.Items)
	}
	after, _ := service.Detail(archive.ID)
	if after.Version != version || len(after.Timeline) != timeline || after.Status != domain.StatusReviewing {
		t.Fatalf("对照查询推进了业务状态: %#v", after)
	}

	_, _ = st.Transact(archive.ID, "tamper-test", func(candidate *domain.Archive) error {
		candidate.CheckRuns[currentResult.Run.ID].FindingsHash = "broken"
		return nil
	})
	_, err = service.ReworkComparison(archive.ID, currentResult.Run.ID, ComparisonQuery{})
	if !errors.Is(err, domain.ErrDigestMismatch) {
		t.Fatalf("损坏的发现摘要未被拒绝: %v", err)
	}
}

func TestCertificateBatchVerificationDeduplicatesAndDoesNotWrite(t *testing.T) {
	st, _ := store.New("")
	service := New(st)
	first := acceptedArchive(t, service, "CERT-BATCH-1", "cert-create-1")
	second := acceptedArchive(t, service, "CERT-BATCH-2", "cert-create-2")
	firstBeforeVersion, firstBeforeTimeline := first.Version, len(first.Timeline)
	result, err := service.VerifyCertificates([]string{" " + first.Certificate.CertificateID + " ", second.Certificate.CertificateID, first.Certificate.CertificateID, "cert-999999"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 || result.Summary.Valid != 2 || result.Summary.NotFound != 1 || result.Items[0].CertificateID != first.Certificate.CertificateID {
		t.Fatalf("批量核验结果错误: %#v", result)
	}
	after, _ := service.Detail(first.ID)
	if after.Version != firstBeforeVersion || len(after.Timeline) != firstBeforeTimeline {
		t.Fatalf("凭据核验产生了写入: %#v", after)
	}
}

func acceptedArchive(t *testing.T, service *Service, code, key string) *domain.Archive {
	t.Helper()
	archive, err := service.Create(code, code+"洞", "2025-03-01", "CGCS2000", key)
	if err != nil {
		t.Fatal(err)
	}
	revision := &domain.Revision{Stations: []domain.Station{{ID: "S1", Name: "入口"}, {ID: "S2", Name: "深处"}}, Legs: []domain.Leg{{ID: "L1", From: "S1", To: "S2", Distance: 5, Azimuth: 90}}, SubmittedBy: "测绘员"}
	archive, _ = service.AddRevisionIdempotent(archive.ID, revision, archive.Version, key+"-revision")
	archive, _ = service.Submit(archive.ID, archive.Version)
	_, _ = service.Check(archive.ID, archive.Version)
	archive, _ = service.Detail(archive.ID)
	archive, _ = service.CompleteReview(archive.ID, archive.Version)
	preview, _ := service.FreezePreview(archive.ID)
	_, _ = service.FreezeChecked(archive.ID, "管理员", archive.Version, preview.PreviewVersion, preview.PreviewHash)
	archive, _ = service.Detail(archive.ID)
	_, err = service.IssueIdempotent(archive.ID, "档案员", archive.Version, key+"-certificate")
	if err != nil {
		t.Fatal(err)
	}
	archive, _ = service.Detail(archive.ID)
	return archive
}
