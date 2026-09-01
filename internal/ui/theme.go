package ui

import (
	"charm.land/lipgloss/v2"
	"github.com/wingitman/streamy/internal/config"
)

// Styles is the themeable style set used by the generated TUI. It is rebuilt
// whenever the resolved theme changes (startup, config reload, or theme picker).
type Styles struct {
	Header       lipgloss.Style
	Text         lipgloss.Style
	Muted        lipgloss.Style
	Panel        lipgloss.Style
	Selected     lipgloss.Style
	BrandDelby   lipgloss.Style
	BrandSoft    lipgloss.Style
	HintKey      lipgloss.Style
	Error        lipgloss.Style
	Success      lipgloss.Style
	Selector     lipgloss.Style
	InputPrompt  lipgloss.Style
	StatusBar    lipgloss.Style
	ParentCrumb  lipgloss.Style
	RootDir      lipgloss.Style
	Clipboard    lipgloss.Style
	PreviewLabel lipgloss.Style
	PreviewTitle lipgloss.Style
	ConfirmBox   lipgloss.Style
}

// NewStyles builds the style set from a resolved theme. Terminal mode keeps
// bold/italic and layout styling but omits explicit colors so the terminal's
// own foreground and background are inherited.
func NewStyles(theme config.ResolvedTheme) Styles {
	colors := theme.Colors
	term := theme.Terminal

	return Styles{
		Header:       themedStyle(themedBackground(lipgloss.NewStyle().Bold(true), colors, term, "header_background", "#1A1A2E"), colors, term, "primary", "#7C9EF0"),
		Text:         themedStyle(lipgloss.NewStyle(), colors, term, "foreground", "#FFFFFF"),
		Muted:        themedStyle(lipgloss.NewStyle(), colors, term, "muted", "#666688"),
		Panel:        themedBorder(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1), colors, term, "border", "#444466"),
		Selected:     themedBackground(themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "selected_foreground", "#EEEEFF"), colors, term, "selected_background", "#cd0fc1"),
		BrandDelby:   themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "brand_primary", "#FFFFFF"),
		BrandSoft:    themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "brand_secondary", "#5865F2"),
		HintKey:      themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "hint_key", "#FFE66D"),
		Error:        themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "error", "#F07C7C"),
		Success:      themedStyle(lipgloss.NewStyle(), colors, term, "success", "#7CF09C"),
		Selector:     themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "selector", "#FFFFFF"),
		InputPrompt:  themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "accent", "#F0A47C"),
		StatusBar:    themedStyle(lipgloss.NewStyle(), colors, term, "muted", "#666688"),
		ParentCrumb:  themedStyle(lipgloss.NewStyle().Italic(true), colors, term, "parent_crumb", "#3A3A5A"),
		RootDir:      themedStyle(lipgloss.NewStyle(), colors, term, "root_directory", "#555577"),
		Clipboard:    themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "clipboard", "#F0E07C"),
		PreviewLabel: themedStyle(lipgloss.NewStyle(), colors, term, "muted", "#666688"),
		PreviewTitle: themedStyle(lipgloss.NewStyle().Bold(true), colors, term, "accent", "#F0A47C"),
		ConfirmBox:   themedBorder(lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2).Margin(1, 0), colors, term, "accent", "#F0A47C"),
	}
}

func themedStyle(style lipgloss.Style, colors map[string]string, terminal bool, key, fallback string) lipgloss.Style {
	if value, ok := themedValue(colors, terminal, key, fallback); ok {
		return style.Foreground(lipgloss.Color(value))
	}
	return style
}

func themedBackground(style lipgloss.Style, colors map[string]string, terminal bool, key, fallback string) lipgloss.Style {
	if value, ok := themedValue(colors, terminal, key, fallback); ok {
		return style.Background(lipgloss.Color(value))
	}
	return style
}

func themedBorder(style lipgloss.Style, colors map[string]string, terminal bool, key, fallback string) lipgloss.Style {
	if value, ok := themedValue(colors, terminal, key, fallback); ok {
		return style.BorderForeground(lipgloss.Color(value))
	}
	return style
}

func themedValue(colors map[string]string, terminal bool, key, fallback string) (string, bool) {
	if value := colors[key]; value != "" {
		return value, true
	}
	if terminal {
		return "", false
	}
	return fallback, fallback != ""
}
