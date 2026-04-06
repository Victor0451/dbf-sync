package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/progress"

	syncer "dbf-sync/sync"
)

// progressModel wraps charm.land/bubbles/v2/progress with ETA and speed calculation.
type progressModel struct {
	progress.Model
	Total int    // Track total separately as v2 model doesn't store it
	Phase string // NEW: Track the current synchronization phase

	// Track timestamps for ETA/speed calculation
	startTime   time.Time
	lastUpdate  time.Time
	LastCurrent int // exported for view access

	// Computed values for display
	ETA   string
	Speed string // e.g., "34,200 rec/s"
}

// newProgressModel creates a new progress model with the given total.
func newProgressModel(total int) progressModel {
	p := progress.New(
		progress.WithFillCharacters('█', '░'),
		progress.WithoutPercentage(),
	)

	m := progressModel{
		Model:       p,
		Total:       total,
		startTime:   time.Now(),
		lastUpdate:  time.Now(),
		LastCurrent: 0,
		ETA:         "calculando...",
		Speed:       "",
	}

	return m
}

// updateProgress processes a ProgressUpdate and computes ETA/speed.
func (m *progressModel) updateProgress(pu syncer.ProgressUpdate) {
	now := time.Now()

	// Update our state
	m.Total = pu.Total
	m.Phase = pu.Phase

	// Calculate speed (records per second)
	elapsed := now.Sub(m.startTime)
	if elapsed > 0 && pu.Current > 0 {
		speed := float64(pu.Current) / elapsed.Seconds()
		m.Speed = fmt.Sprintf("%s rec/s", formatNumber(int(speed)))
	}

	// Calculate ETA
	if pu.Current > 0 && m.Total > pu.Current {
		// Items remaining
		remaining := m.Total - pu.Current

		// Time per item
		timePerItem := now.Sub(m.startTime) / time.Duration(pu.Current)

		// ETA = remaining * timePerItem
		etaDuration := timePerItem * time.Duration(remaining)

		if etaDuration < time.Second {
			m.ETA = "<1s"
		} else if etaDuration < time.Minute {
			m.ETA = fmt.Sprintf("%.0fs", etaDuration.Seconds())
		} else {
			min := int(etaDuration.Minutes())
			sec := int(etaDuration.Seconds()) % 60
			m.ETA = fmt.Sprintf("%dm %ds", min, sec)
		}
	} else if m.Total > 0 && pu.Current >= m.Total {
		m.ETA = "completo"
		m.Speed = ""
	} else if m.Total <= 0 {
		m.ETA = "..."
		m.Speed = ""
	}

	m.lastUpdate = now
	m.LastCurrent = pu.Current
}

// View renders the progress bar with ETA and speed info.
func (m progressModel) View() string {
	percent := 0.0
	if m.Total > 0 {
		percent = float64(m.LastCurrent) / float64(m.Total)
	}
	return m.Model.ViewAs(percent)
}
