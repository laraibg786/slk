package confirmprompt

import "charm.land/lipgloss/v2"

// Styles are the prompt's style definitions. Box is rendered at a width
// the model derives from its own size, so it must not set Width itself.
type Styles struct {
	Box    lipgloss.Style
	Title  lipgloss.Style
	Body   lipgloss.Style
	Footer lipgloss.Style

	// BaseANSI is re-asserted after every SGR reset in the composed
	// content. Nested lipgloss renders emit resets that would otherwise
	// drop the box background for the rest of the line. Empty disables
	// the fixup.
	BaseANSI string
}

// DefaultStyles returns standalone-usable styles for a dark or light
// background. Hosts with a theme of their own should build Styles
// directly instead.
func DefaultStyles(isDark bool) Styles {
	lightDark := lipgloss.LightDark(isDark)

	accent := lightDark(lipgloss.Color("#0B5FFF"), lipgloss.Color("#4A9EFF"))
	text := lightDark(lipgloss.Color("#1A1A1A"), lipgloss.Color("#E0E0E0"))
	muted := lightDark(lipgloss.Color("#6A6A6A"), lipgloss.Color("#888888"))
	bg := lightDark(lipgloss.Color("#FFFFFF"), lipgloss.Color("#1A1A2E"))

	return Styles{
		Box: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent).
			BorderBackground(bg).
			Background(bg).
			Padding(1, 1),
		Title:  lipgloss.NewStyle().Background(bg).Foreground(accent).Bold(true),
		Body:   lipgloss.NewStyle().Background(bg).Foreground(text),
		Footer: lipgloss.NewStyle().Background(bg).Foreground(muted),
	}
}
