package ui

import (
	"fmt"
	"os"
	"time"
)

// ProgressBar provides a simple ANSI-based progress bar
type ProgressBar struct {
	total   int
	current int
	width   int
	start   time.Time
}

// NewProgressBar creates a new progress bar
func NewProgressBar(total int) *ProgressBar {
	if total < 1 {
		total = 1
	}
	return &ProgressBar{
		total: total,
		width: 30,
		start: time.Now(),
	}
}

// Update updates the current progress
func (p *ProgressBar) Update(current int) {
	if current > p.total {
		current = p.total
	}
	p.current = current
	p.render()
}

// Increment increases the current count by 1
func (p *ProgressBar) Increment() {
	p.current++
	if p.current > p.total {
		p.current = p.total
	}
	p.render()
}

// Finish marks the progress bar as complete
func (p *ProgressBar) Finish() {
	p.current = p.total
	p.render()
	fmt.Println() // New line after progress bar
}

// render draws the progress bar in-place
func (p *ProgressBar) render() {
	percentage := float64(p.current) / float64(p.total)
	filled := int(percentage * float64(p.width))
	
	// Calculate ETA
	var eta string
	if p.current > 0 {
		elapsed := time.Since(p.start)
		perItem := elapsed / time.Duration(p.current)
		remaining := perItem * time.Duration(p.total-p.current)
		if remaining < time.Second {
			eta = "<1s"
		} else {
			eta = remaining.Round(time.Second).String()
		}
	} else {
		eta = "?"
	}

	// Build the bar
	bar := ""
	for i := 0; i < p.width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}

	// Clear line and write new progress
	fmt.Printf("\r[%s] %3.0f%% (%d/%d) ETA: %s", bar, percentage*100, p.current, p.total, eta)
	
	// Force output to appear immediately
	os.Stdout.Sync()
}

// GetCurrent returns the current progress count
func (p *ProgressBar) GetCurrent() int {
	return p.current
}

// GetTotal returns the total count
func (p *ProgressBar) GetTotal() int {
	return p.total
}
