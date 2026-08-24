package domain

// HashText provides the canonical SHA-256 representation for a standalone text value.
func HashText(value string) string {
	return Hash(value)
}
