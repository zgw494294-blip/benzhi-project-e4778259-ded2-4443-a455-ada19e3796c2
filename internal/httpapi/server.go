package httpapi

import (
	"cave-archive/internal/application"
	"cave-archive/internal/domain"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

//go:embed web/index.html web/app.js web/style.css
var webFS embed.FS

type Server struct {
	App *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) *Server {
	s := &Server{App: app, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) routes() {
	s.mux.HandleFunc("/", s.index)
	s.mux.HandleFunc("/static/", s.static)
	s.mux.HandleFunc("/api/archives", s.archives)
	s.mux.HandleFunc("/api/archives/search", s.archives)
	s.mux.HandleFunc("/api/archives/", s.archiveAction)
	s.mux.HandleFunc("/api/certificates/", s.certificateVerify)
}
func (s *Server) Handler() http.Handler { return logging(s.mux) }
func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, _ := webFS.ReadFile("web/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b)
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/static/")
	b, e := webFS.ReadFile("web/" + name)
	if e != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".js") {
		w.Header().Set("Content-Type", "text/javascript")
	} else {
		w.Header().Set("Content-Type", "text/css")
	}
	w.Write(b)
}
func decode(r *http.Request, v any) error {
	if r.ContentLength > 1<<20 {
		return errors.New("请求体过大")
	}
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	return dec.Decode(v)
}
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func writeErr(w http.ResponseWriter, e error) {
	code := http.StatusBadRequest
	if errors.Is(e, domain.ErrNotFound) {
		code = 404
	} else if errors.Is(e, domain.ErrConflict) || errors.Is(e, domain.ErrIdempotencyConflict) || errors.Is(e, domain.ErrArchiveCodeConflict) || errors.Is(e, domain.ErrDuplicateContent) || errors.Is(e, domain.ErrDigestMismatch) {
		code = 409
	} else if errors.Is(e, domain.ErrForbidden) {
		code = 422
	}
	body := map[string]any{"error": e.Error()}
	var business *domain.BusinessError
	if errors.As(e, &business) {
		body["code"] = business.Code
		if business.ExistingArchiveID != "" {
			body["existingArchiveId"] = business.ExistingArchiveID
		}
		if business.ExistingRevisionID != "" {
			body["existingRevisionId"] = business.ExistingRevisionID
		}
		if len(business.FindingIDs) > 0 {
			body["findingIds"] = business.FindingIDs
		}
		if business.Field != "" {
			body["field"] = business.Field
		}
	}
	writeJSON(w, code, body)
}
func (s *Server) archives(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		q := r.URL.Query()
		keyword := q.Get("keyword")
		if keyword == "" {
			keyword = q.Get("q")
		}
		from := q.Get("from")
		if from == "" {
			from = q.Get("startDate")
		}
		to := q.Get("to")
		if to == "" {
			to = q.Get("endDate")
		}
		if from == "" {
			from = q.Get("surveyDateFrom")
		}
		if to == "" {
			to = q.Get("surveyDateTo")
		}
		p := 1
		size := 20
		if v, e := strconv.Atoi(q.Get("page")); e == nil && v > 0 {
			p = v
		}
		if v, e := strconv.Atoi(q.Get("pageSize")); e == nil && v > 0 {
			size = v
		}
		status := q.Get("status")
		if status == "" {
			status = q.Get("state")
		}
		validStatus := map[string]bool{"draft": true, "pending_check": true, "reviewing": true, "rework": true, "freezable": true, "frozen": true, "accepted": true}
		if status != "" && !validStatus[status] {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "状态值非法", "field": "status"})
			return
		}
		if from != "" && to != "" && to < from {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "结束日期不能早于开始日期", "field": "to"})
			return
		}
		query := application.ArchiveQuery{Keyword: keyword, Status: status, From: from, To: to, Page: p, PageSize: size}
		result, e := s.App.Search(query)
		if e != nil {
			writeErr(w, e)
			return
		}
		if q.Get("summary") == "true" || q.Get("summary") == "1" {
			summary, err := s.App.Summary(query)
			if err != nil {
				writeErr(w, err)
				return
			}
			result.Summary = &summary
		}
		writeJSON(w, 200, result)
	case "POST":
		var in struct{ ArchiveCode, CaveName, SurveyDate, CoordinateDatum, IdempotencyKey string }
		if e := decode(r, &in); e != nil {
			writeErr(w, e)
			return
		}
		if in.IdempotencyKey == "" {
			in.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
		if len(in.IdempotencyKey) > 128 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "idempotencyKey不能超过128个字符", "field": "idempotencyKey"})
			return
		}
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(in.SurveyDate)); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "测绘日期格式必须为YYYY-MM-DD", "field": "surveyDate"})
			return
		}
		a, e := s.App.Create(in.ArchiveCode, in.CaveName, in.SurveyDate, in.CoordinateDatum, in.IdempotencyKey)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, a)
	default:
		w.WriteHeader(405)
	}
}
func (s *Server) certificateVerify(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/certificates/"), "/")
	if path == "verify-batch" || path == "batch-verify" || path == "batch" || (path == "verify" && r.Method == http.MethodPost) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var in struct {
			CertificateIDs []string `json:"certificateIds"`
			IDs            []string `json:"ids"`
		}
		if err := decode(r, &in); err != nil {
			writeErr(w, err)
			return
		}
		if in.CertificateIDs == nil {
			in.CertificateIDs = in.IDs
		}
		result, err := s.App.VerifyCertificates(in.CertificateIDs)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	id := path
	if id == "" {
		http.NotFound(w, r)
		return
	}
	if !strings.HasPrefix(id, "cert-") {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "凭据编号格式非法", "field": "certificateId"})
		return
	}
	for _, a := range s.App.List() {
		if a.Certificate != nil && a.Certificate.CertificateID == id {
			writeJSON(w, 200, a.VerifyCertificate())
			return
		}
	}
	writeJSON(w, 200, map[string]any{"result": "not_found", "certificateId": id})
}
func parseExpected(r *http.Request) int {
	v, _ := strconv.Atoi(r.Header.Get("X-Expected-Version"))
	if v == 0 {
		v, _ = strconv.Atoi(r.URL.Query().Get("expectedVersion"))
	}
	return v
}
func (s *Server) archiveAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/archives/"), "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == "GET" {
		a, e := s.App.Detail(id)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, a)
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if r.Method != http.MethodGet && action != "certificate" {
		if archive, err := s.App.Detail(id); err == nil && (archive.Status == domain.StatusFrozen || archive.Status == domain.StatusAccepted) {
			writeErr(w, &domain.BusinessError{Cause: domain.ErrForbidden, Code: "archive_frozen", Message: "归档已冻结，不能再修改"})
			return
		}
	}
	switch action {
	case "revisions":
		if len(parts) > 2 && (parts[2] == "validate" || parts[2] == "precheck") {
			archive, e := s.App.Detail(id)
			if e != nil {
				writeErr(w, e)
				return
			}
			if archive.Status != domain.StatusDraft && archive.Status != domain.StatusRework {
				writeErr(w, fmt.Errorf("%w: 只能在草稿或待返修状态预检修订", domain.ErrForbidden))
				return
			}
			var in revisionInput
			if e := decode(r, &in); e != nil {
				writeErr(w, e)
				return
			}
			stationText, legText, tabular := in.tableText()
			if tabular {
				writeJSON(w, http.StatusOK, parseRevisionTables(stationText, legText))
				return
			}
			writeJSON(w, http.StatusOK, s.App.ValidateRevision(&domain.Revision{Stations: in.Stations, Legs: in.Legs}))
			return
		}
		var in revisionInput
		if e := decode(r, &in); e != nil {
			writeErr(w, e)
			return
		}
		stationText, legText, tabular := in.tableText()
		if tabular {
			parsed := parseRevisionTables(stationText, legText)
			if !parsed.Valid {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "表格批量预检失败", "valid": false, "errors": parsed.Errors, "stations": parsed.Stations, "legs": parsed.Legs})
				return
			}
			in.Stations, in.Legs = parsed.Stations, parsed.Legs
		}
		validation := s.App.ValidateRevision(&domain.Revision{Stations: in.Stations, Legs: in.Legs})
		if !validation.Valid {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "批量数据校验失败", "valid": false, "errors": validation.Errors})
			return
		}
		v := parseExpected(r)
		if v == 0 {
			v = in.ExpectedVersion
		}
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			key = strings.TrimSpace(in.IdempotencyKey)
		}
		if len(key) > 128 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Idempotency-Key不能超过128个字符", "field": "Idempotency-Key"})
			return
		}
		revision := &domain.Revision{Stations: in.Stations, Legs: in.Legs, ChangeSummary: in.ChangeSummary, SubmittedBy: in.SubmittedBy, ParentRevisionID: in.ParentRevisionID}
		a, e := s.App.AddRevisionIdempotent(id, revision, v, key)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, http.StatusCreated, struct {
			*domain.Archive
			RevisionID  string `json:"revisionId"`
			ContentHash string `json:"contentHash"`
		}{Archive: a, RevisionID: revision.ID, ContentHash: revision.ContentHash})
	case "submit":
		var in struct {
			ExpectedVersion int `json:"expectedVersion"`
		}
		_ = decode(r, &in)
		v := parseExpected(r)
		if v == 0 {
			v = in.ExpectedVersion
		}
		a, e := s.App.Submit(id, v)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, a)
	case "check-runs":
		if r.Method == "GET" {
			if len(parts) < 3 {
				http.NotFound(w, r)
				return
			}
			if len(parts) >= 4 && (parts[3] == "comparison" || parts[3] == "rework-comparison") {
				q := r.URL.Query()
				report, e := s.App.ReworkComparison(id, parts[2], application.ComparisonQuery{Category: q.Get("category"), Severity: q.Get("severity"), RuleCode: q.Get("ruleCode")})
				if e != nil {
					writeErr(w, e)
					return
				}
				writeJSON(w, http.StatusOK, report)
				return
			}
			if len(parts) >= 4 && parts[3] == "findings" {
				q := r.URL.Query()
				sum, e := s.App.Findings(id, parts[2], application.FindingQuery{Severity: q.Get("severity"), RuleCode: q.Get("ruleCode"), SubjectType: q.Get("subjectType"), Decision: q.Get("decision")})
				if e != nil {
					writeErr(w, e)
					return
				}
				writeJSON(w, 200, sum)
				return
			}
			run, e := s.App.CheckRun(id, parts[2])
			if e != nil {
				writeErr(w, e)
				return
			}
			writeJSON(w, 200, run)
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expectedVersion"`
			RevisionID      string `json:"revisionId"`
			RuleSetVersion  string `json:"ruleSetVersion"`
		}
		_ = decode(r, &in)
		v := parseExpected(r)
		if v == 0 {
			v = in.ExpectedVersion
		}
		if in.RevisionID != "" {
			a, err := s.App.Detail(id)
			if err != nil {
				writeErr(w, err)
				return
			}
			if a.CurrentRevisionID != in.RevisionID {
				writeJSON(w, http.StatusConflict, map[string]any{"error": "只能检查当前待检修订", "code": "non_current_revision", "currentRevisionId": a.CurrentRevisionID})
				return
			}
		}
		res, e := s.App.CheckVersion(id, v, in.RuleSetVersion)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, res)
	case "findings":
		if len(parts) == 2 && r.Method == "GET" {
			q := r.URL.Query()
			runID := q.Get("checkRunId")
			if runID == "" {
				a, e := s.App.Detail(id)
				if e != nil {
					writeErr(w, e)
					return
				}
				for rid, run := range a.CheckRuns {
					if run.RevisionID == a.CurrentRevisionID && (runID == "" || run.CompletedAt > a.CheckRuns[runID].CompletedAt) {
						runID = rid
					}
				}
			}
			sum, e := s.App.Findings(id, runID, application.FindingQuery{Severity: q.Get("severity"), RuleCode: q.Get("ruleCode"), SubjectType: q.Get("subjectType"), Decision: q.Get("decision")})
			if e != nil {
				writeErr(w, e)
				return
			}
			writeJSON(w, 200, sum)
			return
		}
		if len(parts) >= 3 && (parts[2] == "batch" || parts[2] == "batch-decision") {
			var in struct {
				Items           []domain.DecisionInput `json:"items"`
				ExpectedVersion int                    `json:"expectedVersion"`
			}
			if e := decode(r, &in); e != nil {
				writeErr(w, e)
				return
			}
			v := parseExpected(r)
			if v == 0 {
				v = in.ExpectedVersion
			}
			a, e := s.App.BatchDecide(id, in.Items, v)
			if e != nil {
				writeErr(w, e)
				return
			}
			writeJSON(w, 200, a)
			return
		}
		if len(parts) < 4 || parts[3] != "decision" {
			http.NotFound(w, r)
			return
		}
		var in struct {
			Decision        string `json:"decision"`
			Reason          string `json:"reason"`
			Reviewer        string `json:"reviewer"`
			ReviewerRole    string `json:"reviewerRole"`
			SecondReviewer  string `json:"secondReviewer"`
			Rectification   string `json:"rectification"`
			RelatedObject   string `json:"relatedObject"`
			ExpectedVersion int    `json:"expectedVersion"`
		}
		if e := decode(r, &in); e != nil {
			writeErr(w, e)
			return
		}
		v := parseExpected(r)
		if v == 0 {
			v = in.ExpectedVersion
		}
		f, e := s.App.DecideDetailed(id, domain.DecisionInput{FindingID: parts[2], Decision: in.Decision, Reason: in.Reason, Reviewer: in.Reviewer, ReviewerRole: in.ReviewerRole, SecondReviewer: in.SecondReviewer, Rectification: in.Rectification, RelatedObject: in.RelatedObject}, v)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, f)
	case "review":
		if len(parts) >= 3 && parts[2] == "preview" {
			g, e := s.App.ReviewPreview(id)
			if e != nil {
				writeErr(w, e)
				return
			}
			writeJSON(w, 200, g)
			return
		}
		if len(parts) < 3 || parts[2] != "complete" {
			http.NotFound(w, r)
			return
		}
		var in struct {
			ExpectedVersion int `json:"expectedVersion"`
		}
		_ = decode(r, &in)
		v := parseExpected(r)
		if v == 0 {
			v = in.ExpectedVersion
		}
		a, e := s.App.CompleteReview(id, v)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, a)
	case "differences":
		d, e := s.App.DifferenceReport(id)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, d)
	case "rework-comparison", "comparison-report":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		q := r.URL.Query()
		report, e := s.App.ReworkComparison(id, q.Get("checkRunId"), application.ComparisonQuery{Category: q.Get("category"), Severity: q.Get("severity"), RuleCode: q.Get("ruleCode")})
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, http.StatusOK, report)
	case "timeline":
		q := r.URL.Query()
		p, size := 1, 50
		if v, e := strconv.Atoi(q.Get("page")); e == nil && v > 0 {
			p = v
		}
		if v, e := strconv.Atoi(q.Get("pageSize")); e == nil && v > 0 {
			size = v
		}
		tp, e := s.App.Timeline(id, q.Get("type"), q.Get("actor"), q.Get("from"), q.Get("to"), p, size)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, tp)
	case "freeze":
		if len(parts) >= 3 && parts[2] == "preview" {
			p, e := s.App.FreezePreview(id)
			if e != nil {
				writeErr(w, e)
				return
			}
			writeJSON(w, 200, p)
			return
		}
		var in struct {
			FrozenBy        string `json:"frozenBy"`
			ExpectedVersion int    `json:"expectedVersion"`
			PreviewVersion  int    `json:"previewVersion"`
			PreviewHash     string `json:"previewHash"`
		}
		_ = decode(r, &in)
		v := parseExpected(r)
		if v == 0 {
			v = in.ExpectedVersion
		}
		m, e := s.App.FreezeChecked(id, in.FrozenBy, v, in.PreviewVersion, in.PreviewHash)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, m)
	case "certificate":
		if len(parts) >= 3 && (parts[2] == "verify" || (len(parts) >= 4 && parts[3] == "verify")) {
			a, e := s.App.Detail(id)
			if e != nil {
				writeErr(w, e)
				return
			}
			q := r.URL.Query()
			requested := q.Get("certificateId")
			if requested == "" && len(parts) >= 4 {
				requested = parts[2]
			}
			if requested != "" && (a.Certificate == nil || a.Certificate.CertificateID != requested) {
				writeJSON(w, 200, map[string]any{"result": "not_found"})
				return
			}
			writeJSON(w, 200, a.VerifyCertificate())
			return
		}
		var in struct {
			IssuedBy        string `json:"issuedBy"`
			ExpectedVersion int    `json:"expectedVersion"`
		}
		_ = decode(r, &in)
		key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "签发必须提供Idempotency-Key", "field": "Idempotency-Key"})
			return
		}
		if len(key) > 128 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "Idempotency-Key不能超过128个字符", "field": "Idempotency-Key"})
			return
		}
		v := parseExpected(r)
		if v == 0 {
			v = in.ExpectedVersion
		}
		c, e := s.App.IssueIdempotent(id, in.IssuedBy, v, key)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, c)
	default:
		http.NotFound(w, r)
	}
}
