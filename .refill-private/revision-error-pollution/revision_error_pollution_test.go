package revision_error_pollution_test

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedRevisionWriteDoesNotMutateInput(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "events.jsonl")
	st, err := store.New(logPath)
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	archive, err := service.Create("WRITE-FAIL-1", "持久化洞", "2025-01-02", "CGCS2000", "create-write-fail")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(logPath, 0755); err != nil {
		t.Fatal(err)
	}

	revision := &domain.Revision{
		Stations:    []domain.Station{{ID: " S1 ", Name: " 入口 "}},
		SubmittedBy: " 测绘员甲 ",
	}
	_, err = service.AddRevisionIdempotent(archive.ID, revision, archive.Version, "revision-write-fail")
	if err == nil {
		t.Fatal("预期事件日志写入失败")
	}
	if revision.ID != "" || revision.ArchiveID != "" || revision.RevisionNumber != 0 || revision.SubmittedAt != "" || revision.ContentHash != "" {
		t.Fatalf("失败事务污染了调用方修订对象: %#v", revision)
	}
	if revision.Stations[0].ID != " S1 " || revision.SubmittedBy != " 测绘员甲 " {
		t.Fatalf("失败事务提前规范化了调用方数据: %#v", revision)
	}
	stored, err := st.Get(archive.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Revisions) != 0 || stored.CurrentRevisionID != "" || stored.Version != archive.Version {
		t.Fatalf("失败事务改变了存储状态: %#v", stored)
	}
}
