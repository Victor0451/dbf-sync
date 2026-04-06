package metrics

import (
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
)

// HistogramBuckets defines the default histogram bucket boundaries (in seconds).
var HistogramBuckets = []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10}

// counterValue holds the value for a counter metric.
type counterValue struct {
	value atomic.Uint64
}

// gaugeValue holds the value for a gauge metric.
type gaugeValue struct {
	value atomic.Int64
}

// histogramValue holds the values for a histogram metric.
type histogramValue struct {
	count atomic.Uint64
	mu    sync.Mutex
	sum   float64
	// buckets is a map from bucket boundary to cumulative count
	buckets    map[float64]*atomic.Uint64
	bucketList []float64 // sorted list of bucket boundaries for iteration
}

func (h *histogramValue) addSum(v float64) {
	h.mu.Lock()
	h.sum += v
	h.mu.Unlock()
}

func (h *histogramValue) loadSum() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sum
}

func newHistogramValue() *histogramValue {
	buckets := make(map[float64]*atomic.Uint64)
	for _, b := range HistogramBuckets {
		buckets[b] = &atomic.Uint64{}
	}
	return &histogramValue{
		buckets:    buckets,
		bucketList: HistogramBuckets,
	}
}

// labelSet creates a canonical string key for a label set.
// The key is sorted by label keys for consistency.
func labelSetKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	// Sort keys for consistent ordering
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var s string
	for i, k := range keys {
		if i > 0 {
			s += ","
		}
		s += k + "=" + labels[k]
	}
	return s
}

// collector is a thread-safe implementation of Collector.
type collector struct {
	counters   sync.Map // map[metricName+labelSet] -> *counterValue
	gauges     sync.Map // map[metricName+labelSet] -> *gaugeValue
	histograms sync.Map // map[metricName+labelSet] -> *histogramValue
}

// NewCollector creates a new thread-safe collector.
func NewCollector() Collector {
	return &collector{}
}

func (c *collector) getOrCreateCounter(name string, labels map[string]string) *counterValue {
	key := name + "{" + labelSetKey(labels) + "}"
	raw, ok := c.counters.Load(key)
	if ok {
		return raw.(*counterValue)
	}
	// Use LoadOrStore to avoid race condition on creation
	actual, loaded := c.counters.LoadOrStore(key, &counterValue{})
	if loaded {
		return actual.(*counterValue)
	}
	return actual.(*counterValue)
}

func (c *collector) getOrCreateGauge(name string, labels map[string]string) *gaugeValue {
	key := name + "{" + labelSetKey(labels) + "}"
	raw, ok := c.gauges.Load(key)
	if ok {
		return raw.(*gaugeValue)
	}
	actual, loaded := c.gauges.LoadOrStore(key, &gaugeValue{})
	if loaded {
		return actual.(*gaugeValue)
	}
	return actual.(*gaugeValue)
}

func (c *collector) getOrCreateHistogram(name string, labels map[string]string) *histogramValue {
	key := name + "{" + labelSetKey(labels) + "}"
	raw, ok := c.histograms.Load(key)
	if ok {
		return raw.(*histogramValue)
	}
	actual, loaded := c.histograms.LoadOrStore(key, newHistogramValue())
	if loaded {
		return actual.(*histogramValue)
	}
	return actual.(*histogramValue)
}

// IncCounter increments a counter by 1.
func (c *collector) IncCounter(name string, labels map[string]string) {
	counter := c.getOrCreateCounter(name, labels)
	counter.value.Add(1)
}

// AddCounter adds a value to a counter.
func (c *collector) AddCounter(name string, labels map[string]string, value float64) {
	counter := c.getOrCreateCounter(name, labels)
	counter.value.Add(uint64(value))
}

// SetGauge sets a gauge to the given value.
func (c *collector) SetGauge(name string, labels map[string]string, value float64) {
	gauge := c.getOrCreateGauge(name, labels)
	gauge.value.Store(int64(value))
}

// IncGauge increments a gauge by 1.
func (c *collector) IncGauge(name string, labels map[string]string) {
	gauge := c.getOrCreateGauge(name, labels)
	gauge.value.Add(1)
}

// DecGauge decrements a gauge by 1.
func (c *collector) DecGauge(name string, labels map[string]string) {
	gauge := c.getOrCreateGauge(name, labels)
	gauge.value.Add(-1)
}

// ObserveHistogram records an observation in a histogram.
func (c *collector) ObserveHistogram(name string, labels map[string]string, value float64) {
	h := c.getOrCreateHistogram(name, labels)
	h.count.Add(1)
	h.addSum(value)
	// Increment all buckets that are >= value
	for _, boundary := range h.bucketList {
		if value <= boundary {
			h.buckets[boundary].Add(1)
		}
	}
}

// Gather returns all collected metrics as metric families.
func (c *collector) Gather() []MetricFamily {
	var families []MetricFamily

	// Gather counters
	c.counters.Range(func(key, value interface{}) bool {
		name := key.(string)
		// Extract metric name (before the {)
		idx := -1
		for i := len(name) - 1; i >= 0; i-- {
			if name[i] == '{' {
				idx = i
				break
			}
		}
		metricName := name[:idx]
		labelsStr := name[idx+1 : len(name)-1]

		sample := Sample{
			Value: float64(value.(*counterValue).value.Load()),
		}
		if labelsStr != "" {
			sample.Labels = parseLabels(labelsStr)
		}

		families = append(families, MetricFamily{
			Name:   metricName,
			Type:   TypeCounter,
			Help:   metricName + " counter",
			Samples: []Sample{sample},
		})
		return true
	})

	// Gather gauges
	c.gauges.Range(func(key, value interface{}) bool {
		name := key.(string)
		idx := -1
		for i := len(name) - 1; i >= 0; i-- {
			if name[i] == '{' {
				idx = i
				break
			}
		}
		metricName := name[:idx]
		labelsStr := name[idx+1 : len(name)-1]

		sample := Sample{
			Value: float64(value.(*gaugeValue).value.Load()),
		}
		if labelsStr != "" {
			sample.Labels = parseLabels(labelsStr)
		}

		families = append(families, MetricFamily{
			Name:   metricName,
			Type:   TypeGauge,
			Help:   metricName + " gauge",
			Samples: []Sample{sample},
		})
		return true
	})

	// Gather histograms
	c.histograms.Range(func(key, value interface{}) bool {
		name := key.(string)
		idx := -1
		for i := len(name) - 1; i >= 0; i-- {
			if name[i] == '{' {
				idx = i
				break
			}
		}
		metricName := name[:idx]
		labelsStr := name[idx+1 : len(name)-1]

		h := value.(*histogramValue)
		count := h.count.Load()
		sum := h.loadSum()

		sample := Sample{
			Count:   count,
			Sum:     sum,
			Buckets: make(map[float64]uint64),
		}
		if labelsStr != "" {
			sample.Labels = parseLabels(labelsStr)
		}

		// Build buckets map with cumulative counts
		var cumulative uint64
		for _, boundary := range HistogramBuckets {
			cumulative += h.buckets[boundary].Load()
			sample.Buckets[boundary] = cumulative
		}

		families = append(families, MetricFamily{
			Name:   metricName,
			Type:   TypeHistogram,
			Help:   metricName + " histogram",
			Samples: []Sample{sample},
		})
		return true
	})

	return families
}

// parseLabels parses a label string like "foo=bar,baz=qux" into a map.
func parseLabels(s string) map[string]string {
	if s == "" {
		return nil
	}
	labels := make(map[string]string)
	parts := splitLabels(s)
	for _, part := range parts {
		idx := -1
		for i := 0; i < len(part); i++ {
			if part[i] == '=' {
				idx = i
				break
			}
		}
		if idx > 0 {
			key := part[:idx]
			value := part[idx+1:]
			labels[key] = value
		}
	}
	return labels
}

// splitLabels splits a label string by comma, respecting quoted values.
// This is a simplified implementation for the common case.
func splitLabels(s string) []string {
	var result []string
	var current []byte
	inQuote := false

	for i := 0; i < len(s); i++ {
		if s[i] == '"' {
			inQuote = !inQuote
		} else if s[i] == ',' && !inQuote {
			result = append(result, string(current))
			current = nil
		} else {
			current = append(current, s[i])
		}
	}
	if len(current) > 0 {
		result = append(result, string(current))
	}
	return result
}

// formatFloat formats a float for Prometheus output.
// It avoids scientific notation for nice formatting.
func formatFloat(f float64) string {
	if f == float64(int64(f)) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}