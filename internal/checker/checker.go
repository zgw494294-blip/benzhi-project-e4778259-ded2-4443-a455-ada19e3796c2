package checker

import (
	"cave-archive/internal/domain"
	"fmt"
	"math"
	"sort"
	"time"
)

type Result struct {
	CheckRunID string            `json:"checkRunId"`
	Run        *domain.CheckRun  `json:"run"`
	Findings   []*domain.Finding `json:"findings"`
	Reused     bool              `json:"reused"`
}

func Run(a *domain.Archive, r *domain.Revision) *Result {
	result, _ := RunVersion(a, r, "topology-v1")
	return result
}

func RunVersion(a *domain.Archive, r *domain.Revision, ruleSetVersion string) (*Result, error) {
	if ruleSetVersion == "" {
		ruleSetVersion = "topology-v1"
	}
	if ruleSetVersion != "topology-v1" {
		return nil, &domain.BusinessError{Cause: domain.ErrUnsupportedRuleSet, Code: "unsupported_rule_set", Message: "不支持的规则集版本: " + ruleSetVersion}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	run := &domain.CheckRun{ID: fmt.Sprintf("run-%d", time.Now().UnixNano()), ArchiveID: a.ID, RevisionID: r.ID, RuleSetVersion: ruleSetVersion, StartedAt: now, InputHash: domain.CheckInputHash(r, ruleSetVersion)}
	var fs []*domain.Finding
	names := map[string]string{}
	ids := map[string]bool{}
	for _, s := range r.Stations {
		if old, ok := names[s.Name]; ok {
			fs = append(fs, finding(run, "DUPLICATE_STATION", "error", "station", s.ID, fmt.Sprintf("测站名称与%s重复", old)))
		}
		names[s.Name] = s.ID
		ids[s.ID] = true
	}
	degree := map[string]int{}
	adj := map[string][]string{}
	for _, l := range r.Legs {
		if !ids[l.From] || !ids[l.To] {
			fs = append(fs, finding(run, "INVALID_REFERENCE", "error", "leg", l.ID, "测段引用不存在的测站"))
			continue
		}
		degree[l.From]++
		degree[l.To]++
		adj[l.From] = append(adj[l.From], l.To)
		adj[l.To] = append(adj[l.To], l.From)
		if l.Distance <= 0 || l.Distance > 100000 || l.Azimuth < 0 || l.Azimuth >= 360 || l.Inclination < -90 || l.Inclination > 90 {
			fs = append(fs, finding(run, "OUT_OF_RANGE", "error", "leg", l.ID, "距离、方位角或倾角超出允许范围"))
		}
	}
	for _, s := range r.Stations {
		if degree[s.ID] == 0 {
			fs = append(fs, finding(run, "ISOLATED_STATION", "warning", "station", s.ID, "测站未连接任何测段"))
		}
	}
	components := 0
	visited := map[string]bool{}
	for _, s := range r.Stations {
		if visited[s.ID] {
			continue
		}
		components++
		q := []string{s.ID}
		visited[s.ID] = true
		for len(q) > 0 {
			n := q[0]
			q = q[1:]
			for _, x := range adj[n] {
				if !visited[x] {
					visited[x] = true
					q = append(q, x)
				}
			}
		}
	}
	if components > 1 {
		fs = append(fs, finding(run, "DISCONNECTED_COMPONENT", "error", "archive", a.ID, fmt.Sprintf("存在%d个断裂连通分量", components)))
	}
	// Deterministic closure check for legs returning to an earlier station.
	for i, l := range r.Legs {
		if l.From == l.To || (i > 0 && l.To == r.Legs[i-1].From) {
			if math.Abs(l.Distance) > 50000 {
				fs = append(fs, finding(run, "CLOSURE_ERROR", "error", "leg", l.ID, "回路闭合差超过阈值"))
			}
		}
	}
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].RuleCode == fs[j].RuleCode {
			return fs[i].SubjectID < fs[j].SubjectID
		}
		return fs[i].RuleCode < fs[j].RuleCode
	})
	for _, f := range fs {
		run.FindingIDs = append(run.FindingIDs, f.ID)
	}
	run.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if len(fs) == 0 {
		run.Result = "pass"
	} else {
		run.Result = "findings"
	}
	run.FindingsHash, run.SummaryHash = domain.FindingSummaryHash(run.InputHash, run.RuleSetVersion, run.Result, fs)
	run.Consistent = true
	return &Result{CheckRunID: run.ID, Run: run, Findings: fs}, nil
}
func finding(run *domain.CheckRun, rule, severity, typ, subject, msg string) *domain.Finding {
	return &domain.Finding{ID: fmt.Sprintf("finding-%d", time.Now().UnixNano()), CheckRunID: run.ID, RuleCode: rule, Severity: severity, SubjectType: typ, SubjectID: subject, Message: msg}
}
