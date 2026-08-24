package httpapi

import (
	"bytes"
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"cave-archive/internal/store"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTablePrecheckAndAtomicRevisionCreation(t *testing.T) {
	st, _ := store.New("")
	app := application.New(st)
	archive, err := app.Create("IMPORT-1", "导入洞", "2025-01-01", "CGCS2000", "create-import")
	if err != nil {
		t.Fatal(err)
	}
	srv := New(app)
	valid := map[string]any{
		"stationTable": "id\tname\tx\ty\tz\n S1 \t 入口 \t0\t0\t0\nS2\t中段\t1\t2\t3\nS3\t深处\t4\t5\t6",
		"legTable":     "id\tfrom\tto\tdistance\tazimuth\tinclination\nL1\tS1\tS2\t8.5\t90\t2\nL2\tS2\tS3\t6\t180\t-3",
	}
	precheck := requestJSON(t, srv, http.MethodPost, "/api/archives/"+archive.ID+"/revisions/precheck", valid, nil)
	if precheck.Code != http.StatusOK {
		t.Fatalf("预检状态错误: %d %s", precheck.Code, precheck.Body.String())
	}
	var preview tableImportResult
	if err := json.Unmarshal(precheck.Body.Bytes(), &preview); err != nil || !preview.Valid || preview.StationCount != 3 || preview.LegCount != 2 {
		t.Fatalf("预检结果错误: %#v, %v", preview, err)
	}

	invalid := map[string]any{
		"stationTable": "id,name,x,y,z\nS1,入口,0,0,0\nS2,中段,1,2,3",
		"legTable":     "id,from,to,distance,azimuth,inclination\nL1,S1,UNKNOWN,8,90,0\nL2,S1,S2,6,360,0",
	}
	bad := requestJSON(t, srv, http.MethodPost, "/api/archives/"+archive.ID+"/revisions/precheck", invalid, nil)
	var badResult tableImportResult
	_ = json.Unmarshal(bad.Body.Bytes(), &badResult)
	if badResult.Valid || !hasRowError(badResult.Errors, "leg", 2, "to") || !hasRowError(badResult.Errors, "leg", 3, "azimuth") {
		t.Fatalf("来源行错误未正确定位: %#v", badResult.Errors)
	}
	unchanged, _ := app.Detail(archive.ID)
	if unchanged.Version != archive.Version || len(unchanged.Revisions) != 0 || len(unchanged.Timeline) != 1 {
		t.Fatalf("失败预检产生了写入: %#v", unchanged)
	}

	valid["submittedBy"] = "测绘员甲"
	valid["changeSummary"] = "现场表格导入"
	valid["expectedVersion"] = archive.Version
	created := requestJSON(t, srv, http.MethodPost, "/api/archives/"+archive.ID+"/revisions", valid, map[string]string{"Idempotency-Key": "import-revision-1"})
	if created.Code != http.StatusCreated {
		t.Fatalf("创建状态错误: %d %s", created.Code, created.Body.String())
	}
	var response struct {
		RevisionID  string `json:"revisionId"`
		ContentHash string `json:"contentHash"`
		Version     int    `json:"version"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &response)
	detail, _ := app.Detail(archive.ID)
	if response.RevisionID == "" || response.ContentHash == "" || response.Version != detail.Version || len(detail.Revisions) != 1 {
		t.Fatalf("修订创建响应不完整: %#v", response)
	}
}

func TestTablePrecheckRejectsMixedDelimiter(t *testing.T) {
	result := parseRevisionTables("id,name,x,y,z\nS1\t入口\t0\t0\t0", "id,from,to,distance,azimuth,inclination\nL1,S1,S2,1,0,0")
	if result.Valid || !hasRowError(result.Errors, "station", 2, "delimiter") {
		t.Fatalf("混合分隔格式未拒绝: %#v", result.Errors)
	}
}

func requestJSON(t *testing.T, srv *Server, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recorder, req)
	return recorder
}

func hasRowError(errors []domain.RowError, objectType string, row int, field string) bool {
	for _, item := range errors {
		if item.ObjectType == objectType && item.Row == row && item.Field == field {
			return true
		}
	}
	return false
}
