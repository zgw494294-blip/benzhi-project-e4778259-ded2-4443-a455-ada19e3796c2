package certificate_error_leak_test

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestIssueDoesNotReturnCertificateWhenPersistenceFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive-events.jsonl")
	st, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	archive, err := service.Create("ERR-CERT-1", "错误洞", "2025-06-01", "CGCS2000", "create-cert-error")
	if err != nil {
		t.Fatal(err)
	}
	revision := &domain.Revision{
		Stations:    []domain.Station{{ID: "s1", Name: "入口"}, {ID: "s2", Name: "深处"}},
		Legs:        []domain.Leg{{ID: "l1", From: "s1", To: "s2", Distance: 5, Azimuth: 90}},
		SubmittedBy: "测绘员",
	}
	archive, err = service.AddRevision(archive.ID, revision, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	archive, err = service.Submit(archive.ID, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Check(archive.ID, archive.Version); err != nil {
		t.Fatal(err)
	}
	archive, err = service.Detail(archive.ID)
	if err != nil {
		t.Fatal(err)
	}
	archive, err = service.CompleteReview(archive.ID, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := service.FreezePreview(archive.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.FreezeChecked(archive.ID, "档案管理员", archive.Version, preview.PreviewVersion, preview.PreviewHash); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	certificate, err := service.IssueIdempotent(archive.ID, "档案管理员", archive.Version, "issue-cert-error")
	if err == nil {
		t.Fatal("事件日志路径失效时签发不应成功")
	}
	if certificate != nil {
		t.Fatalf("持久化失败不应返回证书: %#v", certificate)
	}
	current, err := service.Detail(archive.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != domain.StatusFrozen || current.Certificate != nil {
		t.Fatalf("失败签发污染了归档状态: status=%s certificate=%#v", current.Status, current.Certificate)
	}
}
