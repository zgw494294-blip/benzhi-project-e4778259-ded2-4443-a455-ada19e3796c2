package checkreuselockcycle

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"testing"
)

func TestRepeatedCheckDoesNotDeadlock(t *testing.T) {
	st, err := store.New("")
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	archive, err := service.Create("LOCK-REUSE-1", "锁复用洞", "2025-08-01", "CGCS2000", "create-lock-reuse")
	if err != nil {
		t.Fatal(err)
	}
	revision := &domain.Revision{
		Stations: []domain.Station{
			{ID: "S1", Name: "入口", X: 0, Y: 0, Z: 0},
			{ID: "S2", Name: "主厅", X: 10, Y: 0, Z: 0},
		},
		Legs: []domain.Leg{
			{ID: "L1", From: "S1", To: "S2", Distance: 10, Azimuth: 90, Inclination: 0},
		},
		SubmittedBy: "测绘员甲",
	}
	archive, err = service.AddRevision(archive.ID, revision, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	archive, err = service.Submit(archive.ID, archive.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.CheckVersion(archive.ID, archive.Version, "topology-v1"); err != nil {
		t.Fatal(err)
	}

	if _, err = service.CheckVersion(archive.ID, archive.Version, "topology-v1"); err != nil {
		t.Fatalf("重复质检应复用已有结果: %v", err)
	}
}
