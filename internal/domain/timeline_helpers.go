package domain

// TimelineCount returns the number of recorded lifecycle events.
func (a *Archive) TimelineCount() int {
	if a == nil {
		return 0
	}
	return len(a.Timeline)
}
