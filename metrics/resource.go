package metrics

import (
	"database/sql"
	"runtime"
	"time"
)

// resourceSampler periodically samples runtime and database resource metrics.
type resourceSampler struct {
	collector Collector
	db        *sql.DB
	done      chan struct{}
}

// StartResourceSampler starts a background goroutine that samples
// runtime memory and database stats every 5 seconds.
func StartResourceSampler(c Collector, db *sql.DB) *resourceSampler {
	s := &resourceSampler{
		collector: c,
		db:        db,
		done:      make(chan struct{}),
	}

	go s.run()
	return s
}

// run samples metrics in a loop until stopped.
func (s *resourceSampler) run() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Sample immediately on start
	s.sample()

	for {
		select {
		case <-ticker.C:
			s.sample()
		case <-s.done:
			return
		}
	}
}

// sample reads current resource metrics and updates gauges.
func (s *resourceSampler) sample() {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Set memory gauges
	s.collector.SetGauge(MetricMemAlloc, nil, float64(memStats.Alloc))
	s.collector.SetGauge(MetricMemSys, nil, float64(memStats.Sys))

	// Set database connection gauge if db is available
	if s.db != nil {
		stats := s.db.Stats()
		s.collector.SetGauge(MetricMySQLOpen, nil, float64(stats.InUse))
	}
}

// Stop stops the resource sampler.
func (s *resourceSampler) Stop() {
	close(s.done)
}