package httpapi

import "strings"

func trimmedHeader(value string) string {
	return strings.TrimSpace(value)
}
