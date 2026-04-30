package main

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// Test 1: Mobile-friendly Path Truncation (OS-Agnostic)
func TestTruncatePath(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{filepath.FromSlash("/short/path"), 40, filepath.FromSlash("/short/path")},
		{filepath.FromSlash("/data/data/com.termux/files/home/clones/project"), 40, filepath.FromSlash(".../clones/project")},
		{filepath.FromSlash("/a/b/c/d/e/f/g"), 10, filepath.FromSlash(".../f/g")}, // Changed maxLen from 15 to 10
	}

	for _, tt := range tests {
		result := truncatePath(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncatePath(%s, %d) = %s; want %s", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

// Test 2: Hardware-aware Worker Calculation
func TestOptimalWorkers(t *testing.T) {
	if w := getOptimalWorkers(10); w != 10 {
		t.Errorf("Expected 10 workers (override), got %d", w)
	}

	autoW := getOptimalWorkers(0)
	if autoW < 4 || autoW > 32 {
		t.Errorf("Auto-calculated workers %d outside bounds (4-32)", autoW)
	}
}

// Test 3: Bubble Tea State Machine Transitions
func TestModelUpdate(t *testing.T) {
	cfg := Config{DefaultWorkspace: "/tmp"}
	m := initialModel(cfg, "/tmp")

	if m.state != StateDashboard {
		t.Errorf("Expected initial state to be StateDashboard, got %v", m.state)
	}

	enterMsg := tea.KeyMsg{Type: tea.KeyEnter}
	newModel, _ := m.Update(enterMsg)
	updatedM := newModel.(mainModel)

	if updatedM.state != StateSwarming {
		t.Errorf("Expected state to transition to StateSwarming, got %v", updatedM.state)
	}

	qMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	_, cmd := updatedM.Update(qMsg)
	
	if cmd == nil {
		t.Error("Expected tea.Quit command on pressing 'q', got nil")
	}
}

// Test 4: Config Fallback Behavior
func TestLoadConfigFallback(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tempHome, ".config"))

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.MaxWorkers != 0 {
		t.Errorf("Expected default MaxWorkers to be 0, got %d", cfg.MaxWorkers)
	}
}
