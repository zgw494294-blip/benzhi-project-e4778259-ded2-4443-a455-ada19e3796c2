package logrotationstale

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLogRotationDoesNotLoseAcknowledgedArchive(t *testing.T) {
	eventPath := filepath.Join(t.TempDir(), "archive-events.jsonl")
	active, err := store.New(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(active)
	if _, err := service.Create("ROT-001", "轮换前洞穴", "2025-03-01", "WGS84", "rotation-before"); err != nil {
		t.Fatalf("创建轮换前归档失败: %v", err)
	}

	rotatedPath := eventPath + ".rotated"
	if err := os.Rename(eventPath, rotatedPath); err != nil {
		t.Fatalf("轮换事件日志失败: %v", err)
	}
	previousEvents, err := os.ReadFile(rotatedPath)
	if err != nil {
		t.Fatalf("读取轮换日志失败: %v", err)
	}
	if err := os.WriteFile(eventPath, previousEvents, 0644); err != nil {
		t.Fatalf("创建轮换后的活动日志失败: %v", err)
	}

	acknowledged, err := service.Create("ROT-002", "轮换后洞穴", "2025-03-02", "WGS84", "rotation-after")
	if err != nil {
		t.Fatalf("轮换后的建档请求未成功: %v", err)
	}
	restarted, err := store.New(eventPath)
	if err != nil {
		t.Fatalf("从活动日志重启 Store 失败: %v", err)
	}
	if _, err := restarted.Get(acknowledged.ID); errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("已确认归档在事件日志轮换后丢失: %s", acknowledged.ID)
	} else if err != nil {
		t.Fatalf("读取已确认归档失败: %v", err)
	}
}
