package metrics

// Collector defines the interface for metrics collection.
// All methods are safe for concurrent use.
type Collector interface {
	// IncCounter increments a counter by 1.
	IncCounter(name string, labels map[string]string)
	// AddCounter adds a value to a counter.
	AddCounter(name string, labels map[string]string, value float64)
	// SetGauge sets a gauge to the given value.
	SetGauge(name string, labels map[string]string, value float64)
	// IncGauge increments a gauge by 1.
	IncGauge(name string, labels map[string]string)
	// DecGauge decrements a gauge by 1.
	DecGauge(name string, labels map[string]string)
	// ObserveHistogram records an observation in a histogram.
	ObserveHistogram(name string, labels map[string]string, value float64)
	// Gather returns all collected metrics as metric families.
	Gather() []MetricFamily
}

// MetricType represents the type of a metric.
type MetricType int

const (
	TypeCounter MetricType = iota
	TypeGauge
	TypeHistogram
)

// MetricFamily represents a group of metrics with the same name.
type MetricFamily struct {
	Name    string
	Type    MetricType
	Help    string
	Samples []Sample
}

// Sample represents a single metric data point.
type Sample struct {
	Labels map[string]string
	Value  float64
	// Histogram-specific fields
	Buckets map[float64]uint64 // upper_bound -> cumulative_count
	Count   uint64
	Sum     float64
}

// Pre-defined metric name constants
const (
	MetricRecordsProcessed = "dbfsync_records_processed_total"
	MetricErrorsTotal      = "dbfsync_errors_total"
	MetricSyncDuration    = "dbfsync_sync_duration_seconds"
	MetricActiveOps       = "dbfsync_active_operations"
	MetricMemAlloc        = "dbfsync_memory_alloc_bytes"
	MetricMemSys          = "dbfsync_memory_sys_bytes"
	MetricMySQLOpen       = "dbfsync_mysql_open_connections"
)

// NopCollector is a no-op collector used when metrics is disabled.
var NopCollector Collector = &nopCollector{}

// nopCollector implements Collector but does nothing.
type nopCollector struct{}

func (n *nopCollector) IncCounter(name string, labels map[string]string) {}
func (n *nopCollector) AddCounter(name string, labels map[string]string, value float64) {}
func (n *nopCollector) SetGauge(name string, labels map[string]string, value float64) {}
func (n *nopCollector) IncGauge(name string, labels map[string]string) {}
func (n *nopCollector) DecGauge(name string, labels map[string]string) {}
func (n *nopCollector) ObserveHistogram(name string, labels map[string]string, value float64) {}
func (n *nopCollector) Gather() []MetricFamily { return nil }