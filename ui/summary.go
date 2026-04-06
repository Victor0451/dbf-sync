package ui

import (
	"fmt"
	"strings"
)

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
