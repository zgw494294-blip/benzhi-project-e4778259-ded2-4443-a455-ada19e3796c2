package httpapi

import "net/http"

func methodIs(r *http.Request, method string) bool {
	return r != nil && r.Method == method
}
