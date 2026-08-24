package checker

import (
	"cave-archive/internal/domain"
	"testing"
)

func TestFindsIsolated(t *testing.T) {
	a, _ := domain.NewArchive("A", "洞穴", "2024", "WGS84")
	r := &domain.Revision{ID: "r", Stations: []domain.Station{{ID: "s", Name: "孤点"}}}
	res := Run(a, r)
	if len(res.Findings) == 0 {
		t.Fatal("expected finding")
	}
}
