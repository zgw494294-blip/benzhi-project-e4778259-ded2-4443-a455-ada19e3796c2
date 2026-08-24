package domain

// IsFrozen reports whether the archive has a signed freeze manifest.
func (a *Archive) IsFrozen() bool {
	return a != nil && a.Manifest != nil && (a.Status == StatusFrozen || a.Status == StatusAccepted)
}

// IsAccepted reports whether the final acceptance certificate has been issued.
func (a *Archive) IsAccepted() bool {
	return a != nil && a.Status == StatusAccepted && a.Certificate != nil
}
