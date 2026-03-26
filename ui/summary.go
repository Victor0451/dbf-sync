package ui

import (
	"fmt"
	"strings"
	"time"
)

// SyncSummary holds the results of a sync operation
type SyncSummary struct {
	Database      string
	Table         string
	Action        string
	Period        string // e.g., "03/2026" for cobrador updates
	RecordsBefore int    // NEW - count before sync
	Inserted      int
	Updated       int
	Skipped       int
	Errors        int
	RecordsAfter  int    // NEW - count after sync
	Duration      time.Duration
}

// Print displays the sync summary in a formatted box
func (s *SyncSummary) Print() {
	// Format duration
	durationStr := s.formatDuration(s.Duration)

	// Format numbers with thousands separator
	beforeStr := formatNumber(s.RecordsBefore)
	afterStr := formatNumber(s.RecordsAfter)

	// Build the box content
	lines := []string{
		"╔══════════════════════════════════════════╗",
		"║           SYNC SUMMARY                     ║",
		"╠══════════════════════════════════════════╣",
		fmt.Sprintf("║  Database:    %-26s ║", s.Database),
		fmt.Sprintf("║  Table:       %-26s ║", s.Table),
		fmt.Sprintf("║  Action:      %-26s ║", s.Action),
	}

	// Add period if present
	if s.Period != "" {
		lines = append(lines, fmt.Sprintf("║  Period:      %-26s ║", s.Period))
	}

	lines = append(lines,
		"╠══════════════════════════════════════════╣",
		fmt.Sprintf("║  %s ANTES:     %-25s ║", "📊", beforeStr+" registros"),
		fmt.Sprintf("║  %s Inserted:  %-25d ║", "✅", s.Inserted),
		fmt.Sprintf("║  %s Updated:   %-25d ║", "🔄", s.Updated),
		fmt.Sprintf("║  %s Skipped:   %-25d ║", "⏭️ ", s.Skipped),
		fmt.Sprintf("║  %s DESPUÉS:   %-25s ║", "📊", afterStr+" registros"),
		fmt.Sprintf("║  %s Errors:    %-25d ║", "❌", s.Errors),
		"╠══════════════════════════════════════════╣",
		fmt.Sprintf("║  Duration:    %-26s ║", durationStr),
		"╚══════════════════════════════════════════╝",
	)

	for _, line := range lines {
		fmt.Println(line)
	}
}

// formatDuration formats duration for display
func (s *SyncSummary) formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.0fms", float64(d.Milliseconds()))
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", m, sec)
}

// formatNumberView formats a number with thousands separator (for views)
func formatNumberView(n int) string {
	s := fmt.Sprintf("%d", n)
	result := ""
	count := 0
	for i := len(s) - 1; i >= 0; i-- {
		if count > 0 && count%3 == 0 {
			result = "," + result
		}
		result = string(s[i]) + result
		count++
	}
	return result
}

// Truncate ensures text fits in the summary box
func truncate(text string, maxLen int) string {
	if len(text) > maxLen {
		return text[:maxLen-3] + "..."
	}
	return text
}

// PrintActionMenu prints the available actions
func PrintActionMenu(actions []string) int {
	fmt.Println()
	fmt.Println("  ? What do you want to do?")
	fmt.Println()
	
	for i, action := range actions {
		fmt.Printf("    %d. %s\n", i+1, action)
	}
	
	return len(actions)
}

// PrintStep prints a step header
func PrintStep(step, description string) {
	fmt.Println()
	fmt.Printf("━━━ %s: %s ━━━\n", step, description)
}

// PrintConfirm prints a confirmation prompt
func PrintConfirm(prompt string) bool {
	fmt.Println()
	fmt.Printf("  ? %s %s", prompt, "(y/N)")
	fmt.Println()
	return false
}

// PrintInfo prints info message
func PrintInfo(format string, args ...interface{}) {
	fmt.Printf("  → "+format+"\n", args...)
}

// PrintError prints error message
func PrintError(format string, args ...interface{}) {
	fmt.Printf("  ✗ "+format+"\n", args...)
}

// PrintWarning prints warning message
func PrintWarning(format string, args ...interface{}) {
	fmt.Printf("  ⚠ "+format+"\n", args...)
}

// PrintSuccess prints success message
func PrintSuccess(format string, args ...interface{}) {
	fmt.Printf("  ✓ "+format+"\n", args...)
}

// BoxPrint prints text in a simple box
func BoxPrint(title, content string) {
	lines := strings.Split(content, "\n")
	maxLen := 0
	for _, line := range lines {
		if len(line) > maxLen {
			maxLen = len(line)
		}
	}
	if len(title) > maxLen {
		maxLen = len(title)
	}
	
	border := strings.Repeat("═", maxLen+2)
	
	fmt.Println()
	fmt.Println("╔" + border + "╗")
	fmt.Printf("║ %s%s║\n", title, strings.Repeat(" ", maxLen-len(title)))
	fmt.Println("╠" + border + "╣")
	for _, line := range lines {
		fmt.Printf("║ %s%s║\n", line, strings.Repeat(" ", maxLen-len(line)))
	}
	fmt.Println("╚" + border + "╝")
}
