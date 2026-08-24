package checker

const (
	// RuleSetTopologyV1 is the stable rule set used by the current workflow.
	RuleSetTopologyV1 = "topology-v1"
)

// SupportedRuleSet keeps rule-set validation in one place for callers and diagnostics.
func SupportedRuleSet(version string) bool {
	return version == RuleSetTopologyV1
}
