package domain

import (
	"fmt"
	"sort"
	"strings"
)

func NormalizeArchiveCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func ArchiveRequestDigest(code, cave, date, datum string) string {
	return Hash(struct {
		ArchiveCode     string `json:"archiveCode"`
		CaveName        string `json:"caveName"`
		SurveyDate      string `json:"surveyDate"`
		CoordinateDatum string `json:"coordinateDatum"`
	}{NormalizeArchiveCode(code), strings.TrimSpace(cave), strings.TrimSpace(date), strings.TrimSpace(datum)})
}

func RevisionRequestDigest(archiveID string, _ int, r *Revision) string {
	if r == nil {
		return ""
	}
	copy := *r
	normalizeRevision(&copy)
	copy.ChangeSummary = strings.TrimSpace(copy.ChangeSummary)
	copy.SubmittedBy = strings.TrimSpace(copy.SubmittedBy)
	stations, legs := StableEntities(&copy)
	return Hash(struct {
		ArchiveID        string    `json:"archiveId"`
		ParentRevisionID string    `json:"parentRevisionId"`
		Stations         []Station `json:"stations"`
		Legs             []Leg     `json:"legs"`
		ChangeSummary    string    `json:"changeSummary"`
		SubmittedBy      string    `json:"submittedBy"`
	}{archiveID, strings.TrimSpace(copy.ParentRevisionID), stations, legs, copy.ChangeSummary, copy.SubmittedBy})
}

func StableEntities(r *Revision) ([]Station, []Leg) {
	if r == nil {
		return []Station{}, []Leg{}
	}
	stations := append([]Station(nil), r.Stations...)
	legs := append([]Leg(nil), r.Legs...)
	for i := range stations {
		stations[i].ID = strings.TrimSpace(stations[i].ID)
		stations[i].Name = strings.TrimSpace(stations[i].Name)
	}
	for i := range legs {
		legs[i].ID = strings.TrimSpace(legs[i].ID)
		legs[i].From = strings.TrimSpace(legs[i].From)
		legs[i].To = strings.TrimSpace(legs[i].To)
	}
	sort.Slice(stations, func(i, j int) bool { return stations[i].ID < stations[j].ID })
	sort.Slice(legs, func(i, j int) bool { return legs[i].ID < legs[j].ID })
	return stations, legs
}

func EntitySnapshotHash(stations []Station, legs []Leg) string {
	return Hash(struct {
		Stations []Station `json:"stations"`
		Legs     []Leg     `json:"legs"`
	}{stations, legs})
}

func ManifestDigest(m *Manifest) string {
	if m == nil {
		return ""
	}
	return Hash(struct {
		ManifestID   string    `json:"manifestId"`
		ArchiveID    string    `json:"archiveId"`
		RevisionID   string    `json:"revisionId"`
		StationCount int       `json:"stationCount"`
		LegCount     int       `json:"legCount"`
		ContentHash  string    `json:"contentHash"`
		EntityHash   string    `json:"entityHash"`
		Stations     []Station `json:"stations"`
		Legs         []Leg     `json:"legs"`
		FrozenBy     string    `json:"frozenBy"`
		FrozenAt     string    `json:"frozenAt"`
	}{m.ManifestID, m.ArchiveID, m.RevisionID, m.StationCount, m.LegCount, m.ContentHash, m.EntityHash, m.Stations, m.Legs, m.FrozenBy, m.FrozenAt})
}

func VerifyManifest(m *Manifest) bool {
	if m == nil || len(m.Stations) != m.StationCount || len(m.Legs) != m.LegCount {
		return false
	}
	if EntitySnapshotHash(m.Stations, m.Legs) != m.EntityHash {
		return false
	}
	return ManifestDigest(m) == m.ManifestHash
}

func ManifestMatchesRevision(m *Manifest, r *Revision) bool {
	if !VerifyManifest(m) || r == nil || r.ID != m.RevisionID || RevisionContentHash(r) != m.ContentHash {
		return false
	}
	stations, legs := StableEntities(r)
	return EntitySnapshotHash(stations, legs) == m.EntityHash
}

func FreezePreviewDigest(p *FreezePreview) string {
	if p == nil {
		return ""
	}
	return Hash(struct {
		ArchiveID      string `json:"archiveId"`
		RevisionID     string `json:"revisionId"`
		PreviewVersion int    `json:"previewVersion"`
		ContentHash    string `json:"contentHash"`
		RecomputedHash string `json:"recomputedHash"`
		EntityHash     string `json:"entityHash"`
		StationCount   int    `json:"stationCount"`
		LegCount       int    `json:"legCount"`
	}{p.ArchiveID, p.RevisionID, p.PreviewVersion, p.ContentHash, p.RecomputedHash, p.EntityHash, p.StationCount, p.LegCount})
}

func CheckInputHash(r *Revision, ruleSetVersion string) string {
	stations, legs := StableEntities(r)
	return Hash(struct {
		RuleSetVersion string    `json:"ruleSetVersion"`
		RevisionID     string    `json:"revisionId"`
		ContentHash    string    `json:"contentHash"`
		Stations       []Station `json:"stations"`
		Legs           []Leg     `json:"legs"`
	}{ruleSetVersion, r.ID, r.ContentHash, stations, legs})
}

func FindingSummaryHash(inputHash, ruleSetVersion, result string, findings []*Finding) (string, string) {
	type item struct {
		RuleCode, Severity, SubjectType, SubjectID, Message string
	}
	items := make([]item, 0, len(findings))
	for _, f := range findings {
		if f != nil {
			items = append(items, item{f.RuleCode, f.Severity, f.SubjectType, f.SubjectID, f.Message})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return fmt.Sprint(items[i]) < fmt.Sprint(items[j])
	})
	findingsHash := Hash(items)
	return findingsHash, Hash(struct {
		InputHash      string `json:"inputHash"`
		RuleSetVersion string `json:"ruleSetVersion"`
		Result         string `json:"result"`
		FindingsHash   string `json:"findingsHash"`
	}{inputHash, ruleSetVersion, result, findingsHash})
}

func CheckRunConsistent(run *CheckRun, findings []*Finding) bool {
	if run == nil {
		return false
	}
	fh, sh := FindingSummaryHash(run.InputHash, run.RuleSetVersion, run.Result, findings)
	return fh == run.FindingsHash && sh == run.SummaryHash
}
