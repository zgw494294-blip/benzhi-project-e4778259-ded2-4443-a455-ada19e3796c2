package store

// ArchiveCount returns a consistent snapshot count for operational diagnostics.
func (s *Store) ArchiveCount() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.archives)
}
