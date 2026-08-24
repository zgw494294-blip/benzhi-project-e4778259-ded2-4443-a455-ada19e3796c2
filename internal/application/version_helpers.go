package application

// EffectiveExpectedVersion normalizes optional optimistic-concurrency input.
func EffectiveExpectedVersion(expected, current int) int {
	if expected > 0 {
		return expected
	}
	return current
}
