// Package confirmprompt provides a centered yes/no confirmation overlay
// for destructive actions.
//
// The model owns its size and renders only its own box; compositing it
// over a backdrop is the caller's job.
package confirmprompt

import tea "charm.land/bubbletea/v2"

// Model is the confirmation overlay.
type Model struct {
	KeyMap KeyMap

	styles    Styles
	width     int
	height    int
	visible   bool
	title     string
	body      string
	onConfirm func() tea.Msg

	cache    string
	cacheKey renderKey
}

// New returns a hidden prompt with default keys and styles.
func New() Model {
	return Model{
		KeyMap: DefaultKeyMap(),
		styles: DefaultStyles(true),
	}
}

// Open shows the prompt. body is rendered as a single-line preview of the
// affected content. onConfirm may be nil for a confirmation with no
// follow-up action; otherwise Update returns it as a tea.Cmd.
func (m *Model) Open(title, body string, onConfirm func() tea.Msg) {
	m.title = title
	m.body = body
	m.onConfirm = onConfirm
	m.visible = true
}

// Close hides the prompt and clears its state.
func (m *Model) Close() {
	m.visible = false
	m.title = ""
	m.body = ""
	m.onConfirm = nil
}

// IsVisible reports whether the prompt is showing.
func (m Model) IsVisible() bool { return m.visible }

// SetSize records the terminal dimensions the box is sized against.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Styles returns the prompt's current styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles replaces the prompt's styles. Styles are not part of the
// render key, so this is what drops the cached frame.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.cache, m.cacheKey = "", renderKey{}
}

// Init implements tea.Model. The prompt has no startup work.
func (m Model) Init() tea.Cmd { return nil }
