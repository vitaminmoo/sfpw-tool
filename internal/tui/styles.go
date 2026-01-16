package tui

import "github.com/charmbracelet/lipgloss"

// Styles contains all the lipgloss styles for the TUI.
type Styles struct {
	// App
	App lipgloss.Style

	// Title
	Title       lipgloss.Style
	TitleBar    lipgloss.Style
	Subtitle    lipgloss.Style

	// Menu
	MenuItem         lipgloss.Style
	MenuItemSelected lipgloss.Style
	MenuItemDim      lipgloss.Style

	// Status bar
	StatusBar     lipgloss.Style
	StatusKey     lipgloss.Style
	StatusValue   lipgloss.Style
	StatusOnline  lipgloss.Style
	StatusOffline lipgloss.Style

	// Content
	Content     lipgloss.Style
	Label       lipgloss.Style
	Value       lipgloss.Style
	Highlight   lipgloss.Style
	Muted       lipgloss.Style
	Error       lipgloss.Style
	Success     lipgloss.Style
	Warning     lipgloss.Style

	// Help
	Help lipgloss.Style
}

// DefaultStyles returns the default color scheme.
func DefaultStyles() Styles {
	subtle := lipgloss.AdaptiveColor{Light: "#D9DCCF", Dark: "#383838"}
	highlight := lipgloss.AdaptiveColor{Light: "#874BFD", Dark: "#7D56F4"}
	special := lipgloss.AdaptiveColor{Light: "#43BF6D", Dark: "#73F59F"}

	return Styles{
		App: lipgloss.NewStyle().
			Padding(1, 2),

		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(highlight).
			Padding(0, 1),

		TitleBar: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#343433", Dark: "#C1C6B2"}).
			Background(subtle).
			Padding(0, 1).
			MarginBottom(1),

		Subtitle: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}),

		MenuItem: lipgloss.NewStyle(),

		MenuItemSelected: lipgloss.NewStyle().
			Foreground(highlight).
			Bold(true),

		MenuItemDim: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}).
			PaddingLeft(4),

		StatusBar: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#343433", Dark: "#C1C6B2"}).
			Background(subtle).
			Padding(0, 1).
			MarginTop(1),

		StatusKey: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#626262"}).
			MarginRight(1),

		StatusValue: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#343433", Dark: "#C1C6B2"}).
			MarginRight(2),

		StatusOnline: lipgloss.NewStyle().
			Foreground(special).
			Bold(true),

		StatusOffline: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")).
			Bold(true),

		Content: lipgloss.NewStyle().
			Padding(1, 0),

		Label: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#626262"}).
			Width(16),

		Value: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#343433", Dark: "#C1C6B2"}),

		Highlight: lipgloss.NewStyle().
			Foreground(highlight).
			Bold(true),

		Muted: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}),

		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF6B6B")),

		Success: lipgloss.NewStyle().
			Foreground(special),

		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFCC00")),

		Help: lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#5C5C5C"}).
			MarginTop(1),
	}
}
