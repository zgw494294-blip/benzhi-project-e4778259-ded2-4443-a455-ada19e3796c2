package domain

import "testing"

func TestArchiveFlow(t *testing.T) {
	a, e := NewArchive("A", "洞穴", "2024-01-01", "WGS84")
	if e != nil {
		t.Fatal(e)
	}
	r := &Revision{Stations: []Station{{ID: "s1", Name: "入口"}, {ID: "s2", Name: "深处"}}, Legs: []Leg{{ID: "l1", From: "s1", To: "s2", Distance: 10, Azimuth: 90, Inclination: 0}}, SubmittedBy: "测绘员"}
	if e = a.AddRevision(r, a.Version); e != nil {
		t.Fatal(e)
	}
	if e = a.Submit(a.Version); e != nil {
		t.Fatal(e)
	}
}
