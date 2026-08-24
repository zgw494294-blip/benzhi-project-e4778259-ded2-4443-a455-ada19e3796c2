package application

import (
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"testing"
)

func TestReworkCanFreezeAfterNewCheck(t *testing.T) {
	st, _ := store.New("")
	s := New(st)
	a, _ := s.Create("A-1", "洞穴", "2024-01-01", "WGS84", "k1")
	r := &domain.Revision{Stations: []domain.Station{{ID: "s1", Name: "入口"}}, SubmittedBy: "测绘员"}
	a, _ = s.AddRevision(a.ID, r, a.Version)
	a, _ = s.Submit(a.ID, a.Version)
	res, _ := s.Check(a.ID, a.Version)
	if len(res.Findings) == 0 {
		t.Fatal("expected check finding")
	}
	_, _ = s.Decide(a.ID, res.Findings[0].ID, "rectify", "补充测段", "校审员", a.Version)
	a, _ = s.Detail(a.ID)
	a, _ = s.CompleteReview(a.ID, a.Version)
	if a.Status != domain.StatusRework {
		t.Fatalf("status %s", a.Status)
	}
	r2 := &domain.Revision{ParentRevisionID: r.ID, Stations: []domain.Station{{ID: "s1", Name: "入口"}, {ID: "s2", Name: "深处"}}, Legs: []domain.Leg{{ID: "l1", From: "s1", To: "s2", Distance: 5, Azimuth: 45, Inclination: 1}}, SubmittedBy: "测绘员"}
	a, _ = s.AddRevision(a.ID, r2, a.Version)
	a, _ = s.Submit(a.ID, a.Version)
	res, _ = s.Check(a.ID, a.Version)
	if len(res.Findings) != 0 {
		t.Fatalf("unexpected findings: %d", len(res.Findings))
	}
	a, _ = s.CompleteReview(a.ID, a.Version)
	if a.Status != domain.StatusFreezable {
		t.Fatalf("expected freezable, got %s", a.Status)
	}
}
