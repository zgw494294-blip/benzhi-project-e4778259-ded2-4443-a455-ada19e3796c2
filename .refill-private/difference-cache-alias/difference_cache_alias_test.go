package differencecachealias_test

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"testing"
)

func TestDifferenceReportCacheIsIsolatedFromCallerMutation(t *testing.T) {
	repository, err := store.New("")
	if err != nil {
		t.Fatalf("创建 Store 失败: %v", err)
	}
	service := application.New(repository)
	archive, err := service.Create("REFILL-DIFF-001", "青岩洞", "2026-08-25", "CGCS2000", "create-diff-cache")
	if err != nil {
		t.Fatalf("创建归档失败: %v", err)
	}

	parent := &domain.Revision{
		Stations: []domain.Station{
			{ID: "S1", Name: "入口", X: 0, Y: 0, Z: 0},
			{ID: "S2", Name: "主厅", X: 10, Y: 0, Z: 0},
		},
		Legs: []domain.Leg{
			{ID: "L1", From: "S1", To: "S2", Distance: 10, Azimuth: 90, Inclination: 0},
		},
		ChangeSummary: "初始测绘",
		SubmittedBy:   "测绘员甲",
	}
	archive, err = service.AddRevision(archive.ID, parent, archive.Version)
	if err != nil {
		t.Fatalf("登记父修订失败: %v", err)
	}
	child := &domain.Revision{
		ParentRevisionID: parent.ID,
		Stations: []domain.Station{
			{ID: "S1", Name: "入口", X: 0, Y: 0, Z: 0},
			{ID: "S2", Name: "主厅复测点", X: 10, Y: 0, Z: 0},
		},
		Legs: []domain.Leg{
			{ID: "L1", From: "S1", To: "S2", Distance: 12, Azimuth: 90, Inclination: 0},
		},
		ChangeSummary: "复测主厅",
		SubmittedBy:   "测绘员乙",
	}
	if _, err = service.AddRevision(archive.ID, child, archive.Version); err != nil {
		t.Fatalf("登记子修订失败: %v", err)
	}

	first, err := service.DifferenceReport(archive.ID)
	if err != nil {
		t.Fatalf("首次查询差异失败: %v", err)
	}
	if len(first.Entities) == 0 || len(first.Entities[0].Fields) == 0 {
		t.Fatalf("测试前提不成立: 未生成字段级差异")
	}
	originalField := first.Entities[0].Fields[0].Field
	first.Entities[0].Fields[0].Field = "caller-polluted-field"

	second, err := service.DifferenceReport(archive.ID)
	if err != nil {
		t.Fatalf("再次查询差异失败: %v", err)
	}
	if got := second.Entities[0].Fields[0].Field; got != originalField {
		t.Fatalf("缓存差异被调用方修改污染: got %q, want %q", got, originalField)
	}
}
