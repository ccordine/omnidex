package datasource

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ChartPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type ChartSpec struct {
	Type     string       `json:"type"`
	Title    string       `json:"title,omitempty"`
	LabelKey string       `json:"label_key,omitempty"`
	ValueKey string       `json:"value_key,omitempty"`
	Series   []ChartPoint `json:"series,omitempty"`
}

func BuildChartSpec(result QueryResult) *ChartSpec {
	if len(result.Rows) == 0 || len(result.Columns) == 0 {
		return nil
	}
	labelKey, valueKey := pickChartKeys(result.Columns, result.Rows)
	if labelKey == "" || valueKey == "" {
		return nil
	}
	series := make([]ChartPoint, 0, minInt(len(result.Rows), 24))
	for _, row := range result.Rows[:minInt(len(result.Rows), 24)] {
		label := stringifyChartLabel(row[labelKey])
		value, ok := numericChartValue(row[valueKey])
		if !ok {
			continue
		}
		series = append(series, ChartPoint{Label: label, Value: value})
	}
	if len(series) == 0 {
		return nil
	}
	chartType := "bar"
	if looksLikeTimeSeries(labelKey, series) {
		chartType = "line"
	} else if len(series) <= 8 && allNonNegative(series) {
		chartType = "pie"
	}
	title := strings.TrimSpace(result.Answer)
	if title == "" {
		title = "Query results"
	}
	return &ChartSpec{
		Type:     chartType,
		Title:    title,
		LabelKey: labelKey,
		ValueKey: valueKey,
		Series:   series,
	}
}

func pickChartKeys(columns []string, rows []map[string]any) (string, string) {
	if len(columns) == 0 {
		return "", ""
	}
	numericCols := []string{}
	labelCols := []string{}
	for _, col := range columns {
		if columnMostlyNumeric(rows, col) {
			numericCols = append(numericCols, col)
			continue
		}
		labelCols = append(labelCols, col)
	}
	if len(numericCols) == 0 {
		return "", ""
	}
	valueKey := numericCols[0]
	if len(numericCols) > 1 {
		sort.SliceStable(numericCols, func(i, j int) bool {
			return variance(rows, numericCols[i]) > variance(rows, numericCols[j])
		})
		valueKey = numericCols[0]
	}
	labelKey := ""
	if len(labelCols) > 0 {
		labelKey = labelCols[0]
	} else if len(columns) > 1 {
		for _, col := range columns {
			if col != valueKey {
				labelKey = col
				break
			}
		}
	}
	if labelKey == "" {
		labelKey = columns[0]
	}
	return labelKey, valueKey
}

func columnMostlyNumeric(rows []map[string]any, col string) bool {
	if len(rows) == 0 {
		return false
	}
	numeric := 0
	for _, row := range rows {
		if _, ok := numericChartValue(row[col]); ok {
			numeric++
		}
	}
	return numeric >= maxInt(1, len(rows)/2)
}

func variance(rows []map[string]any, col string) float64 {
	values := []float64{}
	for _, row := range rows {
		if value, ok := numericChartValue(row[col]); ok {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))
	spread := 0.0
	for _, value := range values {
		diff := value - mean
		spread += diff * diff
	}
	return spread / float64(len(values))
}

func numericChartValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return 0, false
		}
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(text, 64)
		return parsed, err == nil
	default:
		text := strings.TrimSpace(stringifyChartLabel(value))
		if text == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(text, 64)
		return parsed, err == nil
	}
}

func stringifyChartLabel(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case time.Time:
		return typed.UTC().Format("2006-01-02")
	case []byte:
		return string(typed)
	default:
		return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(typed), "\n", " "), "\t", " "))
	}
}

func looksLikeTimeSeries(labelKey string, series []ChartPoint) bool {
	lower := strings.ToLower(labelKey)
	if strings.Contains(lower, "date") || strings.Contains(lower, "time") || strings.Contains(lower, "day") || strings.Contains(lower, "week") || strings.Contains(lower, "month") {
		return len(series) >= 3
	}
	for _, point := range series[:minInt(len(series), 4)] {
		if _, err := time.Parse("2006-01-02", point.Label); err == nil {
			return len(series) >= 3
		}
	}
	return false
}

func allNonNegative(series []ChartPoint) bool {
	for _, point := range series {
		if point.Value < 0 {
			return false
		}
	}
	return true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
