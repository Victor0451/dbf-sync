package metrics

import (
	"bytes"
	"strings"
	"testing"
)

func TestContentType(t *testing.T) {
	ct := ContentType()
	expected := "text/plain; version=0.0.4; charset=utf-8"
	if ct != expected {
		t.Errorf("expected %q, got %q", expected, ct)
	}
}

func TestSerializeEmpty(t *testing.T) {
	var buf bytes.Buffer
	Serialize(&buf, nil)
	if buf.Len() != 0 {
		t.Errorf("expected empty output for nil families")
	}

	buf.Reset()
	Serialize(&buf, []MetricFamily{})
	if buf.Len() != 0 {
		t.Errorf("expected empty output for empty families")
	}
}

func TestSerializeCounter(t *testing.T) {
	families := []MetricFamily{
		{
			Name:   "test_counter",
			Type:   TypeCounter,
			Help:   "A test counter",
			Samples: []Sample{
				{
					Value: 42,
					Labels: map[string]string{"label1": "value1"},
				},
			},
		},
	}

	var buf bytes.Buffer
	Serialize(&buf, families)
	output := buf.String()

	// Check # HELP line
	if !strings.Contains(output, "# HELP test_counter A test counter") {
		t.Errorf("missing # HELP line, got: %s", output)
	}

	// Check # TYPE line
	if !strings.Contains(output, "# TYPE test_counter counter") {
		t.Errorf("missing # TYPE line, got: %s", output)
	}

	// Check metric line with labels
	if !strings.Contains(output, "test_counter{label1=\"value1\"} 42") {
		t.Errorf("missing metric line, got: %s", output)
	}
}

func TestSerializeGauge(t *testing.T) {
	families := []MetricFamily{
		{
			Name:   "test_gauge",
			Type:   TypeGauge,
			Help:   "A test gauge",
			Samples: []Sample{
				{
					Value:  3.14,
					Labels: nil,
				},
			},
		},
	}

	var buf bytes.Buffer
	Serialize(&buf, families)
	output := buf.String()

	// Check metric line without labels
	if !strings.Contains(output, "test_gauge 3.14") {
		t.Errorf("missing gauge metric line, got: %s", output)
	}
}

func TestSerializeHistogram(t *testing.T) {
	families := []MetricFamily{
		{
			Name:   "test_histogram",
			Type:   TypeHistogram,
			Help:   "A test histogram",
			Samples: []Sample{
				{
					Count:   100,
					Sum:     42.5,
					Buckets: map[float64]uint64{0.1: 10, 0.5: 50, 1.0: 80, 5.0: 100},
					Labels:  nil,
				},
			},
		},
	}

	var buf bytes.Buffer
	Serialize(&buf, families)
	output := buf.String()

	// Check # HELP
	if !strings.Contains(output, "# HELP test_histogram A test histogram") {
		t.Errorf("missing # HELP, got: %s", output)
	}

	// Check # TYPE
	if !strings.Contains(output, "# TYPE test_histogram histogram") {
		t.Errorf("missing # TYPE, got: %s", output)
	}

	// Check bucket lines
	if !strings.Contains(output, "test_histogram_bucket{le=\"0.1\"} 10") {
		t.Errorf("missing bucket le=0.1, got: %s", output)
	}
	if !strings.Contains(output, "test_histogram_bucket{le=\"+Inf\"} 100") {
		t.Errorf("missing bucket le=+Inf, got: %s", output)
	}

	// Check sum line
	if !strings.Contains(output, "test_histogram_sum 42.5") {
		t.Errorf("missing sum line, got: %s", output)
	}

	// Check count line
	if !strings.Contains(output, "test_histogram_count 100") {
		t.Errorf("missing count line, got: %s", output)
	}
}

func TestSerializeHistogramWithLabels(t *testing.T) {
	families := []MetricFamily{
		{
			Name:   "test_histogram",
			Type:   TypeHistogram,
			Help:   "A test histogram",
			Samples: []Sample{
				{
					Count:   50,
					Sum:     25.0,
					Buckets: map[float64]uint64{1.0: 30, 5.0: 50},
					Labels:  map[string]string{"endpoint": "/api/data"},
				},
			},
		},
	}

	var buf bytes.Buffer
	Serialize(&buf, families)
	output := buf.String()

	// Check bucket with labels
	if !strings.Contains(output, "test_histogram_bucket{endpoint=\"/api/data\",le=\"1.0\"} 30") {
		t.Errorf("bucket with labels not found, got: %s", output)
	}

	// Check count with labels
	if !strings.Contains(output, "test_histogram_count{endpoint=\"/api/data\"} 50") {
		t.Errorf("count with labels not found, got: %s", output)
	}
}

func TestSerializeLabelEscaping(t *testing.T) {
	families := []MetricFamily{
		{
			Name:   "test_metric",
			Type:   TypeGauge,
			Help:   "Test",
			Samples: []Sample{
				{
					Value: 1,
					Labels: map[string]string{
						"backslash": "path\\to\\file",
						"quote":     `say "hello"`,
						"newline":   "line1\nline2",
						"normal":    "normal_value",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	Serialize(&buf, families)
	output := buf.String()

	// Check escaped backslash
	if !strings.Contains(output, `backslash="path\\to\\file"`) {
		t.Errorf("backslash not escaped, got: %s", output)
	}

	// Check escaped quote
	if !strings.Contains(output, `quote="say \"hello\""`) {
		t.Errorf("quote not escaped, got: %s", output)
	}

	// Check escaped newline
	if !strings.Contains(output, `newline="line1\nline2"`) {
		t.Errorf("newline not escaped, got: %s", output)
	}

	// Normal value should remain unchanged
	if !strings.Contains(output, `normal="normal_value"`) {
		t.Errorf("normal label not found, got: %s", output)
	}
}

func TestSerializeMultipleFamilies(t *testing.T) {
	families := []MetricFamily{
		{
			Name:   "aaa_counter",
			Type:   TypeCounter,
			Help:   "First metric",
			Samples: []Sample{{Value: 1}},
		},
		{
			Name:   "zzz_gauge",
			Type:   TypeGauge,
			Help:   "Second metric",
			Samples: []Sample{{Value: 2}},
		},
	}

	var buf bytes.Buffer
	Serialize(&buf, families)
	output := buf.String()

	// Should be sorted by name
	aaaIdx := strings.Index(output, "aaa_counter")
	zzzIdx := strings.Index(output, "zzz_gauge")
	if aaaIdx > zzzIdx {
		t.Errorf("expected aaa_counter before zzz_gauge, got: %s", output)
	}
}

func TestFormatLabelsEmpty(t *testing.T) {
	result := formatLabels(nil)
	if result != "" {
		t.Errorf("expected empty string for nil labels, got: %q", result)
	}

	result = formatLabels(map[string]string{})
	if result != "" {
		t.Errorf("expected empty string for empty map, got: %q", result)
	}
}

func TestFormatLabelsSorted(t *testing.T) {
	labels := map[string]string{"z": "1", "a": "2", "m": "3"}
	result := formatLabels(labels)

	// Should be sorted alphabetically: a, m, z
	if !strings.Contains(result, "a=\"2\"") {
		t.Errorf("expected sorted a first, got: %s", result)
	}
	if !strings.Contains(result, "m=\"3\"") {
		t.Errorf("expected sorted m second, got: %s", result)
	}
	if !strings.Contains(result, "z=\"1\"") {
		t.Errorf("expected sorted z last, got: %s", result)
	}

	// Check order
	aIdx := strings.Index(result, "a=")
	mIdx := strings.Index(result, "m=")
	zIdx := strings.Index(result, "z=")
	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("labels not sorted: %s", result)
	}
}

func TestEscapeLabelValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal", "normal"},
		{`back\slash`, `back\\slash`},
		{`quote"here`, `quote\"here`},
		{"line\nbreak", "line\nbreak"},
		{"carriage\rreturn", "carriage\rreturn"},
		{"all: \\, \", \n", `all: \\, \", \n`},
	}

	for _, tc := range tests {
		result := escapeLabelValue(tc.input)
		if result != tc.expected {
			t.Errorf("escapeLabelValue(%q): expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}