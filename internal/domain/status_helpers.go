package domain

// IsTerminal reports whether an archive can no longer accept workflow edits.
func (s Status) IsTerminal() bool {
	return s == StatusFrozen || s == StatusAccepted
}

// AllowsRevision reports the states in which a new immutable revision may be recorded.
func (s Status) AllowsRevision() bool {
	return s == StatusDraft || s == StatusRework
}
