package create_persistence_ghost_test

import (
	"cave-archive/internal/application"
	"cave-archive/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedCreateDoesNotPublishInMemoryGhost(t *testing.T) {
	eventPath := filepath.Join(t.TempDir(), "events.jsonl")
	st, err := store.New(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(eventPath, 0o700); err != nil {
		t.Fatal(err)
	}

	service := application.New(st)
	if _, err := service.Create("GHOST-001", "失效路径洞", "2025-05-06", "CGCS2000", "ghost-create-key"); err == nil {
		t.Fatal("事件日志路径失效时建档应返回持久化错误")
	}

	archives := service.List()
	if len(archives) != 0 {
		t.Fatalf("建档失败后内存查询仍返回未持久化归档: %s", archives[0].ID)
	}
	if _, err := service.Create("GHOST-001", "失效路径洞", "2025-05-06", "CGCS2000", "ghost-create-key"); err == nil {
		t.Fatal("建档失败后相同幂等请求不应命中未持久化结果")
	}
}
