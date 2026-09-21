// Package ui implements jellytop's terminal interface: a tabbed admin console
// over the Jellyfin server API.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/NilssonMandola/jellytop/internal/jellyfin"
)

// keyHint is one entry in the footer's contextual key list.
type keyHint struct{ key, desc string }

// pane is one tab of the console. Panes own their data, their cursor and the
// keys specific to them; the root model owns layout, polling and modals.
type pane interface {
	Title() string
	// Interval is how often this pane's data should be re-fetched while it is
	// the active tab.
	Interval() time.Duration
	Load() tea.Cmd
	// Handle processes a message, reporting whether it consumed it. Panes get
	// first refusal on key presses, so they only claim keys they actually use.
	Handle(tea.Msg) (tea.Cmd, bool)
	View(w, h int) string
	Keys() []keyHint
	Summary() string
	Err() error
	MoveCursor(delta int)
	Home()
	End()
}

// --- messages -------------------------------------------------------------

type tickMsg time.Time

type flashMsg struct {
	text  string
	isErr bool
}

type confirmMsg struct {
	question string
	danger   bool
	action   func() tea.Cmd
}

type promptMsg struct {
	title       string
	placeholder string
	action      func(string) tea.Cmd
}

type actionMsg struct {
	text string
	err  error
	then tea.Cmd
}

// flash shows a transient message in the status bar.
func flash(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return flashMsg{text: text, isErr: isErr} }
}

// confirm asks a yes/no question before running action.
func confirm(question string, danger bool, action func() tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		return confirmMsg{question: question, danger: danger, action: action}
	}
}

// prompt collects a line of text before running action.
func prompt(title, placeholder string, action func(string) tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		return promptMsg{title: title, placeholder: placeholder, action: action}
	}
}

// runAction performs a server-side mutation, reporting success or failure in
// the status bar and optionally chaining follow-up commands (usually a reload).
func runAction(success string, fn func(context.Context) error, then ...tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		err := fn(ctx)
		var chain tea.Cmd
		if len(then) > 0 {
			chain = tea.Batch(then...)
		}
		return actionMsg{text: success, err: err, then: chain}
	}
}

// --- model ----------------------------------------------------------------

// Model is the root Bubble Tea model.
type Model struct {
	client *jellyfin.Client
	info   *jellyfin.SystemInfo
	url    string

	panes  []pane
	active int

	width, height int

	lastLoad  map[int]time.Time
	lastErr   error
	flashText string
	flashErr  bool
	flashTill time.Time

	confirming *confirmMsg
	prompting  *promptMsg
	input      textinput.Model
	showHelp   bool
}

// New builds the root model for a server.
func New(c *jellyfin.Client, info *jellyfin.SystemInfo, url string) *Model {
	ti := textinput.New()
	ti.CharLimit = 200

	return &Model{
		client: c,
		info:   info,
		url:    url,
		panes: []pane{
			newSessionsPane(c),
			newActivityPane(c),
			newUsersPane(c),
			newLibrariesPane(c),
			newTasksPane(c),
		},
		lastLoad: map[int]time.Time{},
		input:    ti,
	}
}

func (m *Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.panes)+1)
	for i, p := range m.panes {
		cmds = append(cmds, p.Load())
		m.lastLoad[i] = time.Now()
	}
	cmds = append(cmds, tickCmd())
	return tea.Batch(cmds...)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *Model) current() pane { return m.panes[m.active] }

func (m *Model) setFlash(text string, isErr bool) {
	m.flashText, m.flashErr, m.flashTill = text, isErr, time.Now().Add(5*time.Second)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		if !m.flashTill.IsZero() && time.Now().After(m.flashTill) {
			m.flashText, m.flashTill = "", time.Time{}
		}
		cmds := []tea.Cmd{tickCmd()}
		// Only the visible pane polls, and never behind a modal.
		if m.confirming == nil && m.prompting == nil && !m.showHelp {
			p := m.current()
			if time.Since(m.lastLoad[m.active]) >= p.Interval() {
				m.lastLoad[m.active] = time.Now()
				cmds = append(cmds, p.Load())
			}
		}
		return m, tea.Batch(cmds...)

	case flashMsg:
		m.setFlash(msg.text, msg.isErr)
		return m, nil

	case confirmMsg:
		c := msg
		m.confirming = &c
		return m, nil

	case promptMsg:
		pr := msg
		m.prompting = &pr
		m.input.SetValue("")
		m.input.Placeholder = pr.placeholder
		m.input.Focus()
		return m, textinput.Blink

	case actionMsg:
		if msg.err != nil {
			m.setFlash(msg.err.Error(), true)
			return m, nil
		}
		m.setFlash(msg.text, false)
		return m, msg.then

	case tea.KeyMsg:
		// Runes that arrive in a single read reach us as one KeyRunes message
		// ("jj" rather than two "j"s), which happens when a key repeats fast or
		// input is pasted. Split them so every press counts — except while a
		// text prompt is open, where the whole burst belongs in the input.
		if m.prompting == nil && msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			var cmds []tea.Cmd
			for _, r := range msg.Runes {
				_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			return m, tea.Batch(cmds...)
		}
		return m.handleKey(msg)
	}

	// Data messages go to every pane so background loads still land when the
	// user has switched tabs.
	var cmds []tea.Cmd
	for _, p := range m.panes {
		if cmd, ok := p.Handle(msg); ok && cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	m.lastErr = m.current().Err()
	return m, tea.Batch(cmds...)
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Modals swallow everything.
	if m.confirming != nil {
		switch key {
		case "y", "Y", "enter":
			action := m.confirming.action
			m.confirming = nil
			if action != nil {
				return m, action()
			}
			return m, nil
		case "n", "N", "esc", "q", "ctrl+c":
			m.confirming = nil
			return m, nil
		}
		return m, nil
	}

	if m.prompting != nil {
		switch key {
		case "enter":
			action, value := m.prompting.action, m.input.Value()
			m.prompting = nil
			m.input.Blur()
			if action != nil {
				return m, action(value)
			}
			return m, nil
		case "esc", "ctrl+c":
			m.prompting = nil
			m.input.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	if m.showHelp {
		m.showHelp = false
		return m, nil
	}

	// Panes get first refusal.
	if cmd, ok := m.current().Handle(msg); ok {
		return m, cmd
	}

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "tab", "l", "right":
		m.active = (m.active + 1) % len(m.panes)
		return m, m.maybeLoad()
	case "shift+tab", "h", "left":
		m.active = (m.active - 1 + len(m.panes)) % len(m.panes)
		return m, m.maybeLoad()
	case "r":
		m.lastLoad[m.active] = time.Now()
		return m, m.current().Load()
	case "j", "down":
		m.current().MoveCursor(1)
		return m, nil
	case "k", "up":
		m.current().MoveCursor(-1)
		return m, nil
	case "pgdown", "ctrl+f", " ":
		m.current().MoveCursor(m.contentHeight() - 2)
		return m, nil
	case "pgup", "ctrl+b":
		m.current().MoveCursor(-(m.contentHeight() - 2))
		return m, nil
	case "g", "home":
		m.current().Home()
		return m, nil
	case "G", "end":
		m.current().End()
		return m, nil
	}

	if n := int(key[0]) - '1'; len(key) == 1 && n >= 0 && n < len(m.panes) {
		m.active = n
		return m, m.maybeLoad()
	}
	return m, nil
}

// maybeLoad refreshes the newly selected pane if its data is stale.
func (m *Model) maybeLoad() tea.Cmd {
	p := m.current()
	if time.Since(m.lastLoad[m.active]) < p.Interval() {
		return nil
	}
	m.lastLoad[m.active] = time.Now()
	return p.Load()
}

func (m *Model) contentHeight() int {
	// Six, not five: Bubble Tea's renderer reserves the terminal's final row,
	// so a view exactly `height` lines tall loses its footer off the bottom.
	h := m.height - 6
	if h < 3 {
		h = 3
	}
	return h
}

// --- view -----------------------------------------------------------------

func (m *Model) View() string {
	if m.width == 0 {
		return "starting…"
	}

	body := strings.Join([]string{
		m.viewHeader(),
		m.viewTabs(),
		"",
		m.current().View(m.width, m.contentHeight()),
		m.viewStatus(),
		m.viewKeys(),
	}, "\n")

	switch {
	case m.showHelp:
		return m.overlay(body, m.viewHelp(), styleModal)
	case m.confirming != nil:
		style := styleModal
		if m.confirming.danger {
			style = styleModalDanger
		}
		content := styleText.Render(m.confirming.question) + "\n\n" +
			styleKey.Render("y") + styleMuted.Render(" confirm    ") +
			styleKey.Render("n") + styleMuted.Render(" cancel")
		return m.overlay(body, content, style)
	case m.prompting != nil:
		content := styleTitle.Render(m.prompting.title) + "\n\n" +
			m.input.View() + "\n\n" +
			styleKey.Render("enter") + styleMuted.Render(" send    ") +
			styleKey.Render("esc") + styleMuted.Render(" cancel")
		return m.overlay(body, content, styleModal)
	}
	return body
}

func (m *Model) overlay(body, content string, style lipgloss.Style) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, style.Render(content))
}

func (m *Model) viewHeader() string {
	name, version := "jellyfin", "?"
	if m.info != nil {
		if m.info.ServerName != "" {
			name = m.info.ServerName
		}
		version = m.info.Version
	}
	left := styleTitle.Render("jellytop") + styleMuted.Render(
		fmt.Sprintf("  %s · %s · %s", name, version, m.url))
	right := styleFaint.Render(time.Now().Format("15:04:05"))

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fitStyled(left, m.width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) viewTabs() string {
	parts := make([]string, len(m.panes))
	for i, p := range m.panes {
		label := fmt.Sprintf("%d %s", i+1, p.Title())
		if i == m.active {
			parts[i] = styleTabActive.Render(label)
		} else {
			parts[i] = styleTabInactive.Render(label)
		}
	}
	return fitStyled(strings.Join(parts, " "), m.width)
}

func (m *Model) viewStatus() string {
	if m.flashText != "" {
		style := styleOK
		prefix := "✓ "
		if m.flashErr {
			style, prefix = styleErr, "✗ "
		}
		return fitStyled(style.Render(prefix+m.flashText), m.width)
	}
	if err := m.current().Err(); err != nil {
		return fitStyled(styleErr.Render("✗ "+err.Error()), m.width)
	}

	left := styleStatusBar.Render(m.current().Summary())
	right := styleFaint.Render("loading…")
	if t := m.lastLoad[m.active]; !t.IsZero() {
		right = styleFaint.Render("updated " + relTime(t) + " ago")
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fitStyled(left, m.width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *Model) viewKeys() string {
	hints := append([]keyHint{}, m.current().Keys()...)
	hints = append(hints, keyHint{"tab", "next"}, keyHint{"?", "help"}, keyHint{"q", "quit"})

	parts := make([]string, len(hints))
	for i, h := range hints {
		parts[i] = styleKey.Render(h.key) + styleMuted.Render(" "+h.desc)
	}
	return fitStyled(strings.Join(parts, styleFaint.Render("  ·  ")), m.width)
}

func (m *Model) viewHelp() string {
	section := func(title string, hints []keyHint) string {
		var b strings.Builder
		b.WriteString(styleTitle.Render(title) + "\n")
		for _, h := range hints {
			b.WriteString("  " + styleKey.Render(pad(h.key, 12)) + styleMuted.Render(h.desc) + "\n")
		}
		return b.String()
	}

	global := []keyHint{
		{"1…5", "jump to tab"},
		{"tab / ⇧tab", "cycle tabs"},
		{"j / k", "move cursor"},
		{"g / G", "top / bottom"},
		{"pgup/pgdn", "page"},
		{"r", "refresh now"},
		{"?", "this help"},
		{"q", "quit"},
	}

	perPane := []keyHint{
		{"Sessions", "enter details · s stop playback · m message"},
		{"Activity", "enter details · / filter · n/p page"},
		{"Users", "d enable/disable · a toggle admin"},
		{"Libraries", "S scan all libraries"},
		{"Tasks", "enter run now · x cancel"},
	}

	return section("Global", global) + "\n" + section("Per tab", perPane) +
		"\n" + styleFaint.Render("any key closes this help")
}
