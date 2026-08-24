package application

import (
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"errors"
	"testing"
)

func validRevision() *domain.Revision {
	return &domain.Revision{
		Stations:      []domain.Station{{ID: "s1", Name: "入口"}, {ID: "s2", Name: "主洞"}},
		Legs:          []domain.Leg{{ID: "l1", From: "s1", To: "s2", Distance: 8, Azimuth: 90}},
		ChangeSummary: "首次测绘",
		SubmittedBy:   "测绘员甲",
	}
}

func TestCreateAndRevisionIdempotency(t *testing.T) {
	st, _ := store.New("")
	service := New(st)
	archive, err := service.Create(" CAVE-001 ", " 测试洞 ", "2024-01-01", "CGCS2000", "create-1")
	if err != nil {
		t.Fatal(err)
	}
	retried, err := service.Create("CAVE-001", "测试洞", "2024-01-01", "CGCS2000", "create-1")
	if err != nil || retried.ID != archive.ID || len(retried.Timeline) != 1 {
		t.Fatalf("建档重试未复用首次结果: %#v, %v", retried, err)
	}
	_, err = service.Create("cave-001", "另一洞穴", "2024-01-02", "CGCS2000", "create-2")
	var business *domain.BusinessError
	if !errors.Is(err, domain.ErrArchiveCodeConflict) || !errors.As(err, &business) || business.ExistingArchiveID != archive.ID {
		t.Fatalf("编号冲突未返回已有归档: %v", err)
	}

	first := validRevision()
	archive, err = service.AddRevisionIdempotent(archive.ID, first, archive.Version, "revision-1")
	if err != nil {
		t.Fatal(err)
	}
	version, timeline := archive.Version, len(archive.Timeline)
	retry := validRevision()
	archive, err = service.AddRevisionIdempotent(archive.ID, retry, archive.Version, "revision-1")
	if err != nil || retry.ID != first.ID || archive.Version != version || len(archive.Timeline) != timeline {
		t.Fatalf("修订重试产生了额外写入: %v", err)
	}
	_, err = service.AddRevisionIdempotent(archive.ID, validRevision(), archive.Version, "revision-2")
	if !errors.Is(err, domain.ErrDuplicateContent) {
		t.Fatalf("相同内容未被拒绝: %v", err)
	}
}

func TestCheckReplayFreezeAndCertificateVerification(t *testing.T) {
	st, _ := store.New("")
	service := New(st)
	archive, _ := service.Create("FLOW-1", "流程洞", "2024-03-01", "CGCS2000", "create-flow")
	archive, _ = service.AddRevisionIdempotent(archive.ID, validRevision(), archive.Version, "revision-flow")
	archive, _ = service.Submit(archive.ID, archive.Version)
	checkExpected := archive.Version
	first, err := service.CheckVersion(archive.ID, checkExpected, "topology-v1")
	if err != nil {
		t.Fatal(err)
	}
	timeline := len(archive.Timeline)
	replayed, err := service.CheckVersion(archive.ID, checkExpected, "topology-v1")
	if err != nil || replayed.Run.ID != first.Run.ID || len(archive.Timeline) != timeline {
		t.Fatalf("质检运行未被重放: %v", err)
	}
	archive, err = service.CompleteReview(archive.ID, archive.Version)
	if err != nil || archive.Status != domain.StatusFreezable {
		t.Fatalf("校审完成失败: %v", err)
	}
	preview, err := service.FreezePreview(archive.ID)
	if err != nil || len(preview.Stations) != 2 || preview.PreviewHash == "" {
		t.Fatalf("冻结预览不完整: %#v, %v", preview, err)
	}
	manifest, err := service.FreezeChecked(archive.ID, "管理员", archive.Version, preview.PreviewVersion, preview.PreviewHash)
	if err != nil || !domain.ManifestMatchesRevision(manifest, archive.Revisions[manifest.RevisionID]) {
		t.Fatalf("冻结清单验真失败: %v", err)
	}
	issueExpected := archive.Version
	certificate, err := service.IssueIdempotent(archive.ID, "档案员", issueExpected, "certificate-1")
	if err != nil {
		t.Fatal(err)
	}
	retried, err := service.IssueIdempotent(archive.ID, "档案员", issueExpected, "certificate-1")
	if err != nil || retried.CertificateID != certificate.CertificateID {
		t.Fatalf("凭据签发重试未复用: %v", err)
	}
	detail, _ := service.Detail(archive.ID)
	verification := detail.VerifyCertificate()
	if verification.Result != "valid" || len(verification.TimelineEvents) != 2 {
		t.Fatalf("凭据验真结果错误: %#v", verification)
	}
	summary, err := service.Summary(ArchiveQuery{From: "2024-01-01", To: "2024-12-31"})
	if err != nil || summary.ValidCertificateCount != 1 || summary.ArchiveCount != 1 {
		t.Fatalf("跨归档统计错误: %#v, %v", summary, err)
	}
}

func TestHighRiskWaiverRequiresSecondReviewer(t *testing.T) {
	archive, _ := domain.NewArchive("RISK-1", "风险洞", "2024-04-01", "CGCS2000")
	archive.Status = domain.StatusReviewing
	archive.CurrentRevisionID = "rev-1"
	archive.Revisions["rev-1"] = validRevision()
	archive.CheckRuns["run-1"] = &domain.CheckRun{ID: "run-1", RevisionID: "rev-1", CompletedAt: "2024-04-01T00:00:00Z"}
	archive.Findings["finding-1"] = &domain.Finding{ID: "finding-1", CheckRunID: "run-1", Severity: "error"}
	input := domain.DecisionInput{FindingID: "finding-1", Decision: "waive", Reason: "现场确认", Reviewer: "校审员甲"}
	if err := archive.BatchDecide([]domain.DecisionInput{input}, archive.Version); err == nil {
		t.Fatal("单人豁免error发现应被拒绝")
	}
	input.SecondReviewer = "校审员乙"
	if err := archive.BatchDecide([]domain.DecisionInput{input}, archive.Version); err != nil {
		t.Fatal(err)
	}
}
