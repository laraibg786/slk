package confirmprompt

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/muesli/reflow/truncate"
)

// Box width as a share of the terminal, and the bounds it is clamped to.
const (
	widthPercent = 35
	minWidth     = 40
	maxWidth     = 60
	// chrome is the border and horizontal padding the content sits inside.
	chrome = 4
)

// renderKey is every input to View other than the styles, which
// SetStyles invalidates instead. The key map is carried as its help
// fields rather than the built footer so a cache hit allocates nothing.
type renderKey struct {
	width       int
	title       string
	body        string
	confirmKey  string
	confirmDesc string
	cancelKey   string
	cancelDesc  string
}

func (m Model) key(width int) renderKey {
	confirm, cancel := m.KeyMap.Confirm.Help(), m.KeyMap.Cancel.Help()
	return renderKey{
		width:       width,
		title:       m.title,
		body:        m.body,
		confirmKey:  confirm.Key,
		confirmDesc: confirm.Desc,
		cancelKey:   cancel.Key,
		cancelDesc:  cancel.Desc,
	}
}

func (k renderKey) footer() string {
	return "[" + k.confirmKey + "] " + k.confirmDesc + "   [" + k.cancelKey + "] " + k.cancelDesc
}

// View renders the prompt box, or "" when hidden. The caller composites
// it over the backdrop.
//
// Consecutive frames with unchanged inputs are served from cache: the
// prompt is static while open, and lipgloss measuring and padding a
// bordered box is three orders of magnitude dearer than the comparison.
func (m *Model) View() string {
	if !m.visible {
		return ""
	}

	width := boxWidth(m.width)
	key := m.key(width)
	if m.cache != "" && key == m.cacheKey {
		return m.cache
	}

	content := strings.Join([]string{
		m.styles.Title.Render(m.title),
		m.styles.Body.Render("> " + preview(m.body, width-chrome)),
		m.styles.Footer.Render(key.footer()),
	}, "\n\n")

	m.cache = m.styles.Box.Width(width).Render(reassertBase(content, m.styles.BaseANSI))
	m.cacheKey = key
	return m.cache
}

func boxWidth(termWidth int) int {
	return min(max(termWidth*widthPercent/100, minWidth), maxWidth)
}

// preview flattens body to one line and truncates it to limit columns.
func preview(body string, limit int) string {
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\t", " ")
	if lipgloss.Width(body) > limit {
		return truncate.StringWithTail(body, uint(limit), "…")
	}
	return body
}

// reassertBase re-emits style after each SGR reset. lipgloss v2 emits the
// short form; other ANSI producers may emit the explicit zero form.
func reassertBase(text, style string) string {
	if style == "" {
		return text
	}
	text = strings.ReplaceAll(text, "\x1b[0m", "\x1b[0m"+style)
	return strings.ReplaceAll(text, "\x1b[m", "\x1b[m"+style)
}
