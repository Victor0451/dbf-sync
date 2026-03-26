package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"dbf-sync/config"
)

// keyPress builds a KeyPressMsg for the given text character.
func keyPress(text string) tea.KeyPressMsg {
	r := []rune(text)
	if len(r) == 0 {
		return tea.KeyPressMsg{}
	}
	return tea.KeyPressMsg{Code: r[0], Text: text}
}

func newTestModel(state AppState) *AppModel {
	delegate := list.NewDefaultDelegate()
	listModel := list.New([]list.Item{}, delegate, 0, 0)

	spinnerModel := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle()),
	)

	textInputModel := textinput.New()

	return &AppModel{
		state:     state,
		prevState: StateBrowseFile,
		config:    &config.Config{},
		spinner:   spinnerModel,
		list:      listModel,
		textInput: textInputModel,
		ready:     true,
		width:     80,
		height:    24,
	}
}

func TestProcessingBlocksInput(t *testing.T) {
	keys := []string{"q", "n", "s"}
	for _, key := range keys {
		t.Run("key_"+key, func(t *testing.T) {
			m := newTestModel(StateProcessing)
			newModel, _ := m.Update(keyPress(key))
			got := newModel.(*AppModel).state
			if got != StateProcessing {
				t.Errorf("key %q: expected state to remain StateProcessing, got %v", key, got)
			}
		})
	}

	// Test special keys too
	specialKeys := []struct {
		name string
		msg  tea.Msg
	}{
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}},
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}},
	}
	for _, sk := range specialKeys {
		t.Run(sk.name, func(t *testing.T) {
			m := newTestModel(StateProcessing)
			newModel, _ := m.Update(sk.msg)
			got := newModel.(*AppModel).state
			if got != StateProcessing {
				t.Errorf("key %q: expected state to remain StateProcessing, got %v", sk.name, got)
			}
		})
	}
}

func TestConfirmKeys(t *testing.T) {
	t.Run("n goes back to StateBrowseFile", func(t *testing.T) {
		m := newTestModel(StateConfirm)
		m.action = "insert" // not cobrador, so goBack goes to StateBrowseFile
		newModel, _ := m.Update(keyPress("n"))
		got := newModel.(*AppModel).state
		if got != StateBrowseFile {
			t.Errorf("expected StateBrowseFile, got %v", got)
		}
	})

	t.Run("s goes to StateProcessing", func(t *testing.T) {
		m := newTestModel(StateConfirm)
		newModel, _ := m.Update(keyPress("s"))
		got := newModel.(*AppModel).state
		if got != StateProcessing {
			t.Errorf("expected StateProcessing, got %v", got)
		}
	})
}

func TestQuitKeys(t *testing.T) {
	t.Run("n goes back to prevState", func(t *testing.T) {
		m := newTestModel(StateQuit)
		m.prevState = StateSummary
		newModel, _ := m.Update(keyPress("n"))
		got := newModel.(*AppModel).state
		if got != StateSummary {
			t.Errorf("expected StateSummary (prevState), got %v", got)
		}
	})

	t.Run("s returns quit cmd", func(t *testing.T) {
		m := newTestModel(StateQuit)
		_, cmd := m.Update(keyPress("s"))
		if cmd == nil {
			t.Error("expected a non-nil cmd (tea.Quit) for 's' in StateQuit")
		}
	})
}

func TestFormatNumber(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{0, "0"},
		{999, "999"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := formatNumber(tt.input)
			if got != tt.expected {
				t.Errorf("formatNumber(%d) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"500ms", 500 * time.Millisecond, "500ms"},
		{"1500ms is 1.5s", 1500 * time.Millisecond, "1.5s"},
		{"90s is 1m 30s", 90 * time.Second, "1m 30s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, got, tt.expected)
			}
		})
	}
}
