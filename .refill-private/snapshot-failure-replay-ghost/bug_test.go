package snapshotfailurereplayghost

import (
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedSnapshotDoesNotReplayUnacknowledgedArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archive-events.jsonl")
	st, err := store.New(path)
	if err != nil {
		t.Fatal(err)
	}
	temporary := path + ".snapshot.tmp"
	if err := os.Mkdir(temporary, 0755); err != nil {
		t.Fatal(err)
	}
	first, err := domain.NewArchive("A-SNAPSHOT-FAIL", "洞穴", "2024-01-01", "WGS84")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(first, ""); err == nil {
		t.Fatal("expected the snapshot directory to reject the first commit")
	}
	if err := os.Remove(temporary); err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewArchive("A-SNAPSHOT-OK", "洞穴", "2024-01-02", "WGS84")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Create(second, ""); err != nil {
		t.Fatal(err)
	}
	recovered, err := store.New(path)
	if err != nil {
		t.Fatalf("重启恢复失败: %v", err)
	}
	if _, err := recovered.Get(first.ID); err == nil {
		t.Fatalf("失败建档在重启后变成幽灵归档: %s", first.ID)
	}
	if _, err := recovered.Get(second.ID); err != nil {
		t.Fatalf("第二次已确认建档未恢复: %v", err)
	}
}
