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
	visible   bool
	title     string
	body      string
	onConfirm func() tea.Msg

	cache    string
	cacheKey renderKey
}

// Option configures a Model at construction. Anything that also changes
// at runtime has a setter as well.
type Option func(*Model)

// WithStyles sets the prompt's styles.
func WithStyles(s Styles) Option { return func(m *Model) { m.styles = s } }

// WithKeyMap sets the prompt's key bindings.
func WithKeyMap(k KeyMap) Option { return func(m *Model) { m.KeyMap = k } }

// WithWidth sets the terminal width the box is sized against.
func WithWidth(width int) Option { return func(m *Model) { m.width = width } }

// New returns a hidden prompt with default keys and styles.
func New(opts ...Option) Model {
	m := Model{
		KeyMap: DefaultKeyMap(),
		styles: DefaultStyles(true),
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
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

// SetWidth records the terminal width the box is sized against. The box
// height follows its content, and centering it is the caller's job, so
// the terminal height is not the model's business.
func (m *Model) SetWidth(width int) { m.width = width }

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
