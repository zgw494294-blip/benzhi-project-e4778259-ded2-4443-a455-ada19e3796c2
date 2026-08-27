package findingquerystatealias_test

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"testing"
)

func TestFindingsResponseDoesNotExposeInternalFindingState(t *testing.T) {
	st, err := store.New("")
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	archive, err := service.Create("ALIAS-FINDING-1", "状态隔离洞", "2025-05-01", "CGCS2000", "alias-finding-create")
	if err != nil {
		t.Fatal(err)
	}
	revision := &domain.Revision{
		Stations: []domain.Station{
			{ID: "S1", Name: "入口"},
			{ID: "S2", Name: "主洞"},
			{ID: "S3", Name: "孤立测站"},
		},
		Legs:        []domain.Leg{{ID: "L1", From: "S1", To: "S2", Distance: 8, Azimuth: 90}},
		SubmittedBy: "测绘员甲",
	}
	archive, err = service.AddRevisionIdempotent(archive.ID, revision, archive.Version, "alias-finding-revision")
	if err != nil {
		t.Fatal(err)
	}
	archive, err = service.Submit(archive.ID, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	checked, err := service.Check(archive.ID, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Findings(archive.ID, checked.Run.ID, application.FindingQuery{})
	if err != nil || len(first.Findings) == 0 {
		t.Fatalf("质检发现缺失: %#v, %v", first, err)
	}
	first.Findings[0].Decision = "confirm"
	fresh, err := service.Findings(archive.ID, checked.Run.ID, application.FindingQuery{Decision: "confirm"})
	if err != nil {
		t.Fatal(err)
	}
	if len(fresh.Findings) != 0 {
		t.Fatalf("查询响应修改污染了后续状态: %#v", fresh.Findings)
	}
}
