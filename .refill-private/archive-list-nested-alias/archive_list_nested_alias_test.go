package archivelistalias

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"testing"
)

func TestArchiveListDoesNotExposeNestedState(t *testing.T) {
	st, err := store.New("")
	if err != nil {
		t.Fatal(err)
	}
	service := application.New(st)
	archive, err := service.Create("LIST-ALIAS-1", "列表隔离洞", "2025-05-01", "CGCS2000", "list-create")
	if err != nil {
		t.Fatal(err)
	}
	revision := &domain.Revision{
		Stations:    []domain.Station{{ID: "S1", Name: "入口"}, {ID: "S2", Name: "深处"}},
		Legs:        []domain.Leg{{ID: "L1", From: "S1", To: "S2", Distance: 5, Azimuth: 90}},
		SubmittedBy: "测绘员",
	}
	archive, err = service.AddRevisionIdempotent(archive.ID, revision, archive.Version, "list-revision")
	if err != nil {
		t.Fatal(err)
	}
	listed := service.List()
	if len(listed) != 1 || listed[0].Revisions[revision.ID] == nil {
		t.Fatalf("列表结果缺少修订: %#v", listed)
	}
	listed[0].Revisions[revision.ID].Stations[0].Name = "被列表调用方改写"
	current, err := service.Detail(archive.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := current.Revisions[revision.ID].Stations[0].Name; got != "入口" {
		t.Fatalf("列表查询暴露了嵌套状态，后续详情被污染: %q", got)
	}
}
