package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// Prometheus exposition format version
const prometheusFormatVersion = "text/plain; version=0.0.4; charset=utf-8"

// ContentType returns the Content-Type header for Prometheus metrics.
func ContentType() string {
	return prometheusFormatVersion
}

// Serialize writes the metric families in Prometheus text format v0.0.4.
// It outputs # HELP, # TYPE, and metric lines with labels.
func Serialize(w io.Writer, families []MetricFamily) {
	if len(families) == 0 {
		return
	}

	// Sort families by name for consistent output
	sort.Slice(families, func(i, j int) bool {
		return families[i].Name < families[j].Name
	})

	for _, family := range families {
		// Write # HELP line
		fmt.Fprintf(w, "# HELP %s %s\n", family.Name, family.Help)

		// Write # TYPE line
		typeStr := typeString(family.Type)
		fmt.Fprintf(w, "# TYPE %s %s\n", family.Name, typeStr)

		// Write metric samples
		for _, sample := range family.Samples {
			writeSample(w, family.Name, sample, family.Type)
		}
	}
}

// typeString returns the Prometheus type string for a MetricType.
func typeString(t MetricType) string {
	switch t {
	case TypeCounter:
		return "counter"
	case TypeGauge:
		return "gauge"
	case TypeHistogram:
		return "histogram"
	default:
		return "untyped"
	}
}

// writeSample writes a single metric sample in Prometheus format.
func writeSample(w io.Writer, name string, sample Sample, metricType MetricType) {
	// Handle histogram specially - it has multiple lines per sample
	if metricType == TypeHistogram {
		writeHistogramSample(w, name, sample)
		return
	}

	// Build label part
	labelPart := formatLabels(sample.Labels)

	// Write the metric line
	if labelPart != "" {
		fmt.Fprintf(w, "%s{%s} %s\n", name, labelPart, formatFloat(sample.Value))
	} else {
		fmt.Fprintf(w, "%s %s\n", name, formatFloat(sample.Value))
	}
}

// writeHistogramSample writes histogram samples: buckets, sum, and count.
func writeHistogramSample(w io.Writer, name string, sample Sample) {
	// Write bucket lines (sorted by boundary)
	bounds := make([]float64, 0, len(sample.Buckets))
	for b := range sample.Buckets {
		bounds = append(bounds, b)
	}
	sort.Float64s(bounds)

	for _, boundary := range bounds {
		count := sample.Buckets[boundary]
		// Add le label to existing labels
		bucketLabels := addLabel(sample.Labels, "le", formatFloat(boundary))
		bucketLabelPart := formatLabels(bucketLabels)
		fmt.Fprintf(w, "%s_bucket{%s} %d\n", name, bucketLabelPart, count)
	}

	// Write +Inf bucket (cumulative count = total)
	infLabels := addLabel(sample.Labels, "le", "+Inf")
	infLabelPart := formatLabels(infLabels)
	fmt.Fprintf(w, "%s_bucket{%s} %d\n", name, infLabelPart, sample.Count)

	// Write sum line
	if len(sample.Labels) > 0 {
		sumLabelPart := formatLabels(sample.Labels)
		fmt.Fprintf(w, "%s_sum{%s} %s\n", name, sumLabelPart, formatFloat(sample.Sum))
	} else {
		fmt.Fprintf(w, "%s_sum %s\n", name, formatFloat(sample.Sum))
	}

	// Write count line
	if len(sample.Labels) > 0 {
		countLabelPart := formatLabels(sample.Labels)
		fmt.Fprintf(w, "%s_count{%s} %d\n", name, countLabelPart, sample.Count)
	} else {
		fmt.Fprintf(w, "%s_count %d\n", name, sample.Count)
	}
}

// formatLabels formats a map of labels into Prometheus label format.
// Keys are sorted for consistent output.
func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	// Sort keys
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		v := escapeLabelValue(labels[k])
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", k, v))
	}

	return strings.Join(parts, ",")
}

// escapeLabelValue escapes special characters in label values.
// Prometheus requires escaping of \, ", and newlines.
func escapeLabelValue(s string) string {
	// Order matters: escape backslashes first, then quotes, then newlines
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return s
}

// addLabel creates a new label map with an additional label.
func addLabel(labels map[string]string, key, value string) map[string]string {
	if labels == nil {
		labels = make(map[string]string)
	}
	result := make(map[string]string)
	for k, v := range labels {
		result[k] = v
	}
	result[key] = value
	return result
}