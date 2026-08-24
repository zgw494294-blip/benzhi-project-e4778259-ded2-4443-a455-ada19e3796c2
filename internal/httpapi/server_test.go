package httpapi

import (
	"bytes"
	"cave-archive/internal/application"
	"cave-archive/internal/store"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndQueryArchive(t *testing.T) {
	st, _ := store.New("")
	srv := New(application.New(st))
	body := []byte(`{"archiveCode":"T-1","caveName":"测试洞","surveyDate":"2024-02-01","coordinateDatum":"CGCS2000"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/archives", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("unexpected status %d", rec.Code)
	}
	var a map[string]any
	if json.Unmarshal(rec.Body.Bytes(), &a) != nil || a["id"] == nil {
		t.Fatal("response missing id")
	}
	get := httptest.NewRequest(http.MethodGet, "/api/archives/"+a["id"].(string), nil)
	out := httptest.NewRecorder()
	srv.Handler().ServeHTTP(out, get)
	if out.Code != http.StatusOK {
		t.Fatalf("query status %d", out.Code)
	}
}
