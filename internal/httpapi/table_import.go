package httpapi

import (
	"cave-archive/internal/domain"
	"encoding/csv"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
)

type tableImportResult struct {
	Valid            bool              `json:"valid"`
	Errors           []domain.RowError `json:"errors"`
	Stations         []domain.Station  `json:"stations"`
	Legs             []domain.Leg      `json:"legs"`
	StationCount     int               `json:"stationCount"`
	LegCount         int               `json:"legCount"`
	RowCount         int               `json:"rowCount"`
	StationDelimiter string            `json:"stationDelimiter,omitempty"`
	LegDelimiter     string            `json:"legDelimiter,omitempty"`
}

type revisionInput struct {
	Stations         []domain.Station `json:"stations"`
	Legs             []domain.Leg     `json:"legs"`
	StationTable     *string          `json:"stationTable"`
	LegTable         *string          `json:"legTable"`
	StationsTable    *string          `json:"stationsTable"`
	LegsTable        *string          `json:"legsTable"`
	StationText      *string          `json:"stationText"`
	LegText          *string          `json:"legText"`
	ChangeSummary    string           `json:"changeSummary"`
	SubmittedBy      string           `json:"submittedBy"`
	ParentRevisionID string           `json:"parentRevisionId"`
	ExpectedVersion  int              `json:"expectedVersion"`
	IdempotencyKey   string           `json:"idempotencyKey"`
}

func (input revisionInput) tableText() (string, string, bool) {
	station := firstText(input.StationTable, input.StationsTable, input.StationText)
	leg := firstText(input.LegTable, input.LegsTable, input.LegText)
	return valueOf(station), valueOf(leg), station != nil || leg != nil
}

func firstText(values ...*string) *string {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func valueOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type parsedTable struct {
	rows       [][]string
	sourceRows []int
	header     map[string]int
	delimiter  rune
	errors     []domain.RowError
}

var tableHeaders = map[string]map[string]string{
	"station": {
		"id": "id", "stationid": "id", "station_id": "id", "测站编号": "id",
		"name": "name", "stationname": "name", "station_name": "name", "测站名称": "name",
		"x": "x", "y": "y", "z": "z",
	},
	"leg": {
		"id": "id", "legid": "id", "leg_id": "id", "测段编号": "id",
		"from": "from", "fromstation": "from", "from_station": "from", "起点": "from",
		"to": "to", "tostation": "to", "to_station": "to", "终点": "to",
		"distance": "distance", "距离": "distance",
		"azimuth": "azimuth", "方位角": "azimuth",
		"inclination": "inclination", "倾角": "inclination",
	},
}

var requiredHeaders = map[string][]string{
	"station": {"id", "name", "x", "y", "z"},
	"leg":     {"id", "from", "to", "distance", "azimuth", "inclination"},
}

func parseRevisionTables(stationText, legText string) tableImportResult {
	stations := parseTable(stationText, "station")
	legs := parseTable(legText, "leg")
	result := tableImportResult{
		Stations:         make([]domain.Station, 0, len(stations.rows)),
		Legs:             make([]domain.Leg, 0, len(legs.rows)),
		StationDelimiter: delimiterName(stations.delimiter),
		LegDelimiter:     delimiterName(legs.delimiter),
	}
	result.Errors = append(result.Errors, stations.errors...)
	result.Errors = append(result.Errors, legs.errors...)

	for i, row := range stations.rows {
		line := stations.sourceRows[i]
		station := domain.Station{ID: field(row, stations.header, "id"), Name: field(row, stations.header, "name")}
		station.X = numberField(row, stations.header, "x", "station", line, &result.Errors)
		station.Y = numberField(row, stations.header, "y", "station", line, &result.Errors)
		station.Z = numberField(row, stations.header, "z", "station", line, &result.Errors)
		result.Stations = append(result.Stations, station)
	}
	for i, row := range legs.rows {
		line := legs.sourceRows[i]
		leg := domain.Leg{
			ID:   field(row, legs.header, "id"),
			From: field(row, legs.header, "from"),
			To:   field(row, legs.header, "to"),
		}
		leg.Distance = numberField(row, legs.header, "distance", "leg", line, &result.Errors)
		leg.Azimuth = numberField(row, legs.header, "azimuth", "leg", line, &result.Errors)
		leg.Inclination = numberField(row, legs.header, "inclination", "leg", line, &result.Errors)
		result.Legs = append(result.Legs, leg)
	}

	result.StationCount, result.LegCount = len(result.Stations), len(result.Legs)
	result.RowCount = result.StationCount + result.LegCount
	if result.RowCount == 0 {
		result.Errors = append(result.Errors, domain.RowError{ObjectType: "revision", Row: 0, Field: "rows", Reason: "导入数据集不能为空"})
	}
	if result.RowCount > 2000 {
		result.Errors = append(result.Errors, domain.RowError{ObjectType: "revision", Row: 0, Field: "rows", Reason: "批量行数不能超过2000"})
	}
	validation := domain.ValidateRevision(&domain.Revision{Stations: result.Stations, Legs: result.Legs})
	for _, item := range validation.Errors {
		if item.Row > 0 {
			source := stations.sourceRows
			if item.ObjectType == "leg" {
				source = legs.sourceRows
			}
			item.Reason = sourceRowReason(item.Reason, source)
			if item.Row <= len(source) {
				item.Row = source[item.Row-1]
			}
		}
		result.Errors = appendUniqueRowError(result.Errors, item)
	}
	result.Errors = uniqueRowErrors(result.Errors)
	result.Valid = len(result.Errors) == 0
	return result
}

func sourceRowReason(reason string, source []int) string {
	const prefix = "首次出现在第"
	start := strings.Index(reason, prefix)
	if start < 0 {
		return reason
	}
	numberStart := start + len(prefix)
	end := strings.Index(reason[numberStart:], "行")
	if end < 0 {
		return reason
	}
	row, err := strconv.Atoi(reason[numberStart : numberStart+end])
	if err != nil || row < 1 || row > len(source) {
		return reason
	}
	return reason[:numberStart] + strconv.Itoa(source[row-1]) + reason[numberStart+end:]
}

func parseTable(text, objectType string) parsedTable {
	result := parsedTable{header: map[string]int{}}
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		result.errors = append(result.errors, domain.RowError{ObjectType: objectType, Row: 1, Field: "header", Reason: "表格内容不能为空"})
		return result
	}
	firstLine := strings.SplitN(text, "\n", 2)[0]
	commas, tabs := separators(firstLine)
	if commas > 0 && tabs > 0 {
		result.errors = append(result.errors, domain.RowError{ObjectType: objectType, Row: 1, Field: "delimiter", Reason: "表格不能混合使用逗号和制表符分隔"})
		return result
	}
	if commas == 0 && tabs == 0 {
		result.errors = append(result.errors, domain.RowError{ObjectType: objectType, Row: 1, Field: "header", Reason: "无法识别表头或分隔格式"})
		return result
	}
	result.delimiter = ','
	if tabs > 0 {
		result.delimiter = '\t'
	}
	other := '\t'
	if result.delimiter == '\t' {
		other = ','
	}
	for i, line := range strings.Split(text, "\n") {
		_, alternate := separators(line)
		if other == ',' {
			alternate, _ = separators(line)
		}
		if alternate > 0 {
			result.errors = append(result.errors, domain.RowError{ObjectType: objectType, Row: i + 1, Field: "delimiter", Reason: "表格不能混合使用逗号和制表符分隔"})
			return result
		}
	}

	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = result.delimiter
	reader.FieldsPerRecord = -1
	records := [][]string{}
	lines := []int{}
	for {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			line := 1
			var parseErr *csv.ParseError
			if errors.As(err, &parseErr) {
				line = parseErr.Line
			}
			result.errors = append(result.errors, domain.RowError{ObjectType: objectType, Row: line, Field: "row", Reason: "表格格式无法解析"})
			return result
		}
		if blankRecord(record) {
			continue
		}
		line, _ := reader.FieldPos(0)
		for i := range record {
			record[i] = strings.TrimSpace(record[i])
		}
		records = append(records, record)
		lines = append(lines, line)
	}
	if len(records) == 0 {
		result.errors = append(result.errors, domain.RowError{ObjectType: objectType, Row: 1, Field: "header", Reason: "无法识别表头"})
		return result
	}
	aliases := tableHeaders[objectType]
	for i, raw := range records[0] {
		name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff")))
		canonical := aliases[name]
		if canonical == "" {
			continue
		}
		if _, exists := result.header[canonical]; exists {
			result.errors = append(result.errors, domain.RowError{ObjectType: objectType, Row: lines[0], Field: canonical, Reason: "表头列重复"})
			continue
		}
		result.header[canonical] = i
	}
	for _, required := range requiredHeaders[objectType] {
		if _, ok := result.header[required]; !ok {
			result.errors = append(result.errors, domain.RowError{ObjectType: objectType, Row: lines[0], Field: required, Reason: "表头缺少必填列"})
		}
	}
	result.rows = records[1:]
	result.sourceRows = lines[1:]
	return result
}

func field(row []string, header map[string]int, name string) string {
	index, ok := header[name]
	if !ok || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func numberField(row []string, header map[string]int, name, objectType string, line int, out *[]domain.RowError) float64 {
	raw := field(row, header, name)
	if raw == "" {
		*out = append(*out, domain.RowError{ObjectType: objectType, Row: line, Field: name, Reason: "数值不能为空"})
		return 0
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		*out = append(*out, domain.RowError{ObjectType: objectType, Row: line, Field: name, Reason: "非法数值"})
		return 0
	}
	return value
}

func separators(line string) (commas, tabs int) {
	quoted := false
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '"':
			if quoted && i+1 < len(line) && line[i+1] == '"' {
				i++
				continue
			}
			quoted = !quoted
		case ',':
			if !quoted {
				commas++
			}
		case '\t':
			if !quoted {
				tabs++
			}
		}
	}
	return commas, tabs
}

func blankRecord(record []string) bool {
	for _, value := range record {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func delimiterName(delimiter rune) string {
	if delimiter == '\t' {
		return "tab"
	}
	if delimiter == ',' {
		return "comma"
	}
	return ""
}

func appendUniqueRowError(items []domain.RowError, candidate domain.RowError) []domain.RowError {
	for _, item := range items {
		if item.ObjectType == candidate.ObjectType && item.Row == candidate.Row && item.Field == candidate.Field {
			return items
		}
	}
	return append(items, candidate)
}

func uniqueRowErrors(items []domain.RowError) []domain.RowError {
	out := make([]domain.RowError, 0, len(items))
	for _, item := range items {
		out = appendUniqueRowError(out, item)
	}
	return out
}
