package tui

import (
	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ProgressState tracks an ongoing operation with progress.
type ProgressState struct {
	progress    progress.Model
	percent     float64
	description string
	isActive    bool
}

// NewProgressState creates a new progress tracking state.
func NewProgressState() ProgressState {
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(40),
	)
	return ProgressState{
		progress: p,
	}
}

// Start begins tracking a new operation.
func (p *ProgressState) Start(description string) {
	p.isActive = true
	p.percent = 0
	p.description = description
}

// Update updates the progress percentage (0.0 to 1.0).
func (p *ProgressState) Update(percent float64, description string) {
	p.percent = percent
	if description != "" {
		p.description = description
	}
}

// Complete marks the operation as complete.
func (p *ProgressState) Complete() {
	p.percent = 1.0
	p.isActive = false
}

// Cancel stops the progress without completing.
func (p *ProgressState) Cancel() {
	p.isActive = false
}

// IsActive returns whether an operation is in progress.
func (p *ProgressState) IsActive() bool {
	return p.isActive
}

// View renders the progress bar.
func (p ProgressState) View() string {
	if !p.isActive {
		return ""
	}
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	return descStyle.Render(p.description) + "\n" + p.progress.ViewAs(p.percent)
}

// Progress update messages for async operations

// progressUpdateMsg reports progress during an operation.
type progressUpdateMsg struct {
	percent     float64
	description string
	operation   string // identifies which operation this is for
}

// progressCompleteMsg signals an operation completed.
type progressCompleteMsg struct {
	operation string
	success   bool
	message   string
}

// progressErrorMsg signals an operation failed.
type progressErrorMsg struct {
	operation string
	err       error
}

// progressCmd creates a tea.Cmd that sends progress updates.
// This is useful for wrapping operations that report progress via callback.
func progressCmd(operation string, percent float64, description string) tea.Cmd {
	return func() tea.Msg {
		return progressUpdateMsg{
			operation:   operation,
			percent:     percent,
			description: description,
		}
	}
}
