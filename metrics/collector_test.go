package metrics

import (
	"reflect"
	"testing"
)

func TestCounterIncrement(t *testing.T) {
	c := NewCollector()

	// Test IncCounter
	c.IncCounter(MetricRecordsProcessed, map[string]string{"database": "primary", "table": "pagos"})
	c.IncCounter(MetricRecordsProcessed, map[string]string{"database": "primary", "table": "pagos"})
	c.IncCounter(MetricRecordsProcessed, map[string]string{"database": "primary", "table": "pagos"})

	families := c.Gather()
	if len(families) != 1 {
		t.Fatalf("expected 1 family, got %d", len(families))
	}

	f := families[0]
	if f.Name != MetricRecordsProcessed {
		t.Errorf("expected name %s, got %s", MetricRecordsProcessed, f.Name)
	}
	if f.Type != TypeCounter {
		t.Errorf("expected type %v, got %v", TypeCounter, f.Type)
	}
	if len(f.Samples) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(f.Samples))
	}

	s := f.Samples[0]
	if s.Value != 3 {
		t.Errorf("expected value 3, got %v", s.Value)
	}
	if s.Labels["database"] != "primary" {
		t.Errorf("expected database=primary, got %s", s.Labels["database"])
	}
	if s.Labels["table"] != "pagos" {
		t.Errorf("expected table=pagos, got %s", s.Labels["table"])
	}
}

func TestCounterAdd(t *testing.T) {
	c := NewCollector()

	// Test AddCounter
	c.AddCounter(MetricRecordsProcessed, map[string]string{"database": "test"}, 100)
	c.AddCounter(MetricRecordsProcessed, map[string]string{"database": "test"}, 50)

	families := c.Gather()
	if len(families) != 1 {
		t.Fatalf("expected 1 family, got %d", len(families))
	}

	s := families[0].Samples[0]
	if s.Value != 150 {
		t.Errorf("expected value 150, got %v", s.Value)
	}
}

func TestGaugeSet(t *testing.T) {
	c := NewCollector()

	// Test SetGauge
	c.SetGauge(MetricActiveOps, nil, 5)

	families := c.Gather()
	if len(families) != 1 {
		t.Fatalf("expected 1 family, got %d", len(families))
	}

	f := families[0]
	if f.Type != TypeGauge {
		t.Errorf("expected type %v, got %v", TypeGauge, f.Type)
	}

	s := f.Samples[0]
	if s.Value != 5 {
		t.Errorf("expected value 5, got %v", s.Value)
	}
}

func TestGaugeInc(t *testing.T) {
	c := NewCollector()

	c.SetGauge(MetricActiveOps, nil, 0)
	c.IncGauge(MetricActiveOps, nil)
	c.IncGauge(MetricActiveOps, nil)
	c.IncGauge(MetricActiveOps, nil)

	families := c.Gather()
	s := families[0].Samples[0]
	if s.Value != 3 {
		t.Errorf("expected value 3, got %v", s.Value)
	}
}

func TestGaugeDec(t *testing.T) {
	c := NewCollector()

	c.SetGauge(MetricActiveOps, nil, 5)
	c.DecGauge(MetricActiveOps, nil)
	c.DecGauge(MetricActiveOps, nil)

	families := c.Gather()
	s := families[0].Samples[0]
	if s.Value != 3 {
		t.Errorf("expected value 3, got %v", s.Value)
	}
}

func TestHistogramBucketPlacement(t *testing.T) {
	c := NewCollector()

	// Observe values across different buckets
	c.ObserveHistogram(MetricSyncDuration, nil, 0.001)  // < 0.01 (inf)
	c.ObserveHistogram(MetricSyncDuration, nil, 0.02)  // in (0.01, 0.05]
	c.ObserveHistogram(MetricSyncDuration, nil, 0.07)   // in (0.05, 0.1]
	c.ObserveHistogram(MetricSyncDuration, nil, 0.3)    // in (0.1, 0.5]
	c.ObserveHistogram(MetricSyncDuration, nil, 0.8)    // in (0.5, 1]
	c.ObserveHistogram(MetricSyncDuration, nil, 2.0)    // in (1, 5]
	c.ObserveHistogram(MetricSyncDuration, nil, 7.0)    // in (5, 10]
	c.ObserveHistogram(MetricSyncDuration, nil, 15.0)  // > 10 (+Inf)

	families := c.Gather()
	if len(families) != 1 {
		t.Fatalf("expected 1 family, got %d", len(families))
	}

	f := families[0]
	if f.Type != TypeHistogram {
		t.Errorf("expected type %v, got %v", TypeHistogram, f.Type)
	}

	s := f.Samples[0]

	// Count should be 8
	if s.Count != 8 {
		t.Errorf("expected count 8, got %d", s.Count)
	}

	// Check bucket distribution
	// 0.001 should go to all buckets (cumulative)
	// 0.02 should go to buckets >= 0.05
	// etc.
	expectedBuckets := map[float64]uint64{
		0.01: 1, // only 0.001
		0.05: 2, // 0.001 + 0.02
		0.1:  3, // + 0.07
		0.5:  4, // + 0.3
		1.0:  5, // + 0.8
		5.0:  6, // + 2.0
		10.0: 7, // + 7.0
	}

	for boundary, expected := range expectedBuckets {
		actual, ok := s.Buckets[boundary]
		if !ok {
			t.Errorf("bucket %v not found", boundary)
			continue
		}
		if actual != expected {
			t.Errorf("bucket %v: expected %d, got %d", boundary, expected, actual)
		}
	}
}

func TestHistogramSum(t *testing.T) {
	c := NewCollector()

	c.ObserveHistogram(MetricSyncDuration, nil, 1.5)
	c.ObserveHistogram(MetricSyncDuration, nil, 2.5)
	c.ObserveHistogram(MetricSyncDuration, nil, 3.0)

	families := c.Gather()
	s := families[0].Samples[0]

	if s.Count != 3 {
		t.Errorf("expected count 3, got %d", s.Count)
	}
	if s.Sum != 7.0 {
		t.Errorf("expected sum 7.0, got %v", s.Sum)
	}
}

func TestGatherEmpty(t *testing.T) {
	c := NewCollector()

	families := c.Gather()
	if families != nil {
		t.Errorf("expected nil families, got %v", families)
	}
}

func TestMultipleMetrics(t *testing.T) {
	c := NewCollector()

	c.IncCounter(MetricRecordsProcessed, map[string]string{"table": "a"})
	c.SetGauge(MetricActiveOps, nil, 1)
	c.ObserveHistogram(MetricSyncDuration, nil, 1.0)

	families := c.Gather()
	if len(families) != 3 {
		t.Fatalf("expected 3 families, got %d", len(families))
	}

	// Should have counter, gauge, histogram
	types := make(map[MetricType]bool)
	for _, f := range families {
		types[f.Type] = true
	}
	if !types[TypeCounter] {
		t.Error("missing counter family")
	}
	if !types[TypeGauge] {
		t.Error("missing gauge family")
	}
	if !types[TypeHistogram] {
		t.Error("missing histogram family")
	}
}

func TestLabelsEmpty(t *testing.T) {
	c := NewCollector()

	c.IncCounter(MetricRecordsProcessed, nil)

	families := c.Gather()
	if len(families) != 1 {
		t.Fatalf("expected 1 family, got %d", len(families))
	}

	s := families[0].Samples[0]
	if s.Labels != nil {
		t.Errorf("expected nil labels, got %v", s.Labels)
	}
}

func TestNopCollectorSafeNoOp(t *testing.T) {
	// NopCollector should be safe to call with any inputs
	var nc Collector = NopCollector

	// These should not panic
	nc.IncCounter("any", nil)
	nc.AddCounter("any", nil, 1.0)
	nc.SetGauge("any", nil, 1.0)
	nc.IncGauge("any", nil)
	nc.DecGauge("any", nil)
	nc.ObserveHistogram("any", nil, 1.0)

	// Gather should return nil
	result := nc.Gather()
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}

	// Should handle labels gracefully
	nc.IncCounter("test", map[string]string{"key": "value"})
	nc.AddCounter("test", map[string]string{"key": "value"}, 42)
	nc.SetGauge("test", map[string]string{"key": "value"}, 42)
	nc.IncGauge("test", map[string]string{"key": "value"})
	nc.DecGauge("test", map[string]string{"key": "value"})
	nc.ObserveHistogram("test", map[string]string{"key": "value"}, 1.0)
}

func TestHistogramBucketsConstant(t *testing.T) {
	// Verify the bucket boundaries are as specified in design
	expected := []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10}
	if !reflect.DeepEqual(HistogramBuckets, expected) {
		t.Errorf("expected buckets %v, got %v", expected, HistogramBuckets)
	}
}

func TestCounterWithLabels(t *testing.T) {
	c := NewCollector()

	// Same metric with different labels should be tracked separately
	c.IncCounter(MetricRecordsProcessed, map[string]string{"database": "db1", "table": "t1"})
	c.IncCounter(MetricRecordsProcessed, map[string]string{"database": "db1", "table": "t2"})
	c.IncCounter(MetricRecordsProcessed, map[string]string{"database": "db2", "table": "t1"})

	families := c.Gather()
	if len(families) != 1 {
		t.Fatalf("expected 1 family, got %d", len(families))
	}

	if len(families[0].Samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(families[0].Samples))
	}
}

func TestGaugeWithLabels(t *testing.T) {
	c := NewCollector()

	c.SetGauge(MetricActiveOps, map[string]string{"table": "pagos"}, 1)
	c.SetGauge(MetricActiveOps, map[string]string{"table": "pagobco"}, 2)

	families := c.Gather()
	if len(families[0].Samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(families[0].Samples))
	}
}

func TestConcurrentAccess(t *testing.T) {
	c := NewCollector()

	// Run concurrent operations - should not panic or have race conditions
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				c.IncCounter(MetricRecordsProcessed, nil)
				c.AddCounter(MetricErrorsTotal, nil, 1)
				c.SetGauge(MetricActiveOps, nil, 1)
				c.IncGauge(MetricActiveOps, nil)
				c.DecGauge(MetricActiveOps, nil)
				c.ObserveHistogram(MetricSyncDuration, nil, 0.1)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify we get reasonable results
	families := c.Gather()
	if len(families) != 3 {
		t.Fatalf("expected 3 families, got %d", len(families))
	}

	// At least verify gather doesn't panic
	_ = families
}