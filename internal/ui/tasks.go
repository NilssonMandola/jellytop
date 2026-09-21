package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NilssonMandola/jellytop/internal/jellyfin"
	tea "github.com/charmbracelet/bubbletea"
)

type tasksMsg struct {
	items []jellyfin.Task
	err   error
}

type tasksPane struct {
	client *jellyfin.Client
	items  []jellyfin.Task
	cur    cursor
	err    error
}

func newTasksPane(c *jellyfin.Client) *tasksPane { return &tasksPane{client: c} }

func (p *tasksPane) Title() string { return "Tasks" }

// Tasks poll faster while one is running so the progress bar actually moves.
func (p *tasksPane) Interval() time.Duration {
	for _, t := range p.items {
		if t.Running() {
			return 2 * time.Second
		}
	}
	return 20 * time.Second
}

func (p *tasksPane) Err() error { return p.err }

func (p *tasksPane) Load() tea.Cmd {
	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		items, err := c.Tasks(ctx)
		return tasksMsg{items: items, err: err}
	}
}

func (p *tasksPane) Summary() string {
	running := 0
	for _, t := range p.items {
		if t.Running() {
			running++
		}
	}
	return fmt.Sprintf("%d tasks · %d running", len(p.items), running)
}

func (p *tasksPane) Keys() []keyHint {
	return []keyHint{
		{"enter", "run now"},
		{"x", "cancel"},
		{"r", "refresh"},
	}
}

func (p *tasksPane) selected() (jellyfin.Task, bool) {
	if p.cur.idx < 0 || p.cur.idx >= len(p.items) {
		return jellyfin.Task{}, false
	}
	return p.items[p.cur.idx], true
}

func (p *tasksPane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tasksMsg:
		p.err = msg.err
		if msg.err == nil {
			p.items = msg.items
			p.cur.move(0, len(p.items))
		}
		return nil, true

	case tea.KeyMsg:
		t, ok := p.selected()
		if !ok {
			return nil, false
		}
		c := p.client
		switch msg.String() {
		case "enter":
			if t.Running() {
				return flash(t.Name+" is already running", true), true
			}
			return confirm(fmt.Sprintf("Run %q now?", t.Name), false, func() tea.Cmd {
				return runAction("started "+t.Name, func(ctx context.Context) error {
					return c.StartTask(ctx, t.ID)
				}, p.Load())
			}), true
		case "x":
			if !t.Running() {
				return flash(t.Name+" is not running", true), true
			}
			return confirm(fmt.Sprintf("Cancel running task %q?", t.Name), true, func() tea.Cmd {
				return runAction("cancelled "+t.Name, func(ctx context.Context) error {
					return c.StopTask(ctx, t.ID)
				}, p.Load())
			}), true
		}
	}
	return nil, false
}

func (p *tasksPane) MoveCursor(delta int) { p.cur.move(delta, len(p.items)) }
func (p *tasksPane) Home()                { p.cur.toTop() }
func (p *tasksPane) End()                 { p.cur.toEnd(len(p.items)) }

func (p *tasksPane) View(w, h int) string {
	if len(p.items) == 0 {
		return emptyState("No scheduled tasks.", w, h)
	}

	const (
		wName  = 34
		wState = 11
		wBar   = 12
		wLast  = 22
	)
	wCat := w - (wName + wState + wBar + wLast + 4)
	if wCat < 8 {
		wCat = 8
	}

	var b strings.Builder
	b.WriteString(renderRow(false,
		col("TASK", wName, styleHeaderRow), col("STATE", wState, styleHeaderRow),
		col("", wBar, styleHeaderRow), col("LAST RUN", wLast, styleHeaderRow),
		col("CATEGORY", wCat, styleHeaderRow),
	))
	b.WriteString("\n")

	lo, hi := p.cur.window(len(p.items), h-1)
	for i := lo; i < hi; i++ {
		t := p.items[i]

		state := col(strings.ToLower(t.State), wState, styleFaint)
		bar := col("", wBar, styleFaint)
		if t.Running() {
			state = col(strings.ToLower(t.State), wState, styleOK)
			frac := 0.0
			if t.CurrentProgressPct != nil {
				frac = *t.CurrentProgressPct / 100
			}
			plain, styled := progressBar(frac, wBar, styleOK)
			bar = cell{text: plain, w: wBar, style: styleOK, raw: styled}
		}

		last := col("never", wLast, styleFaint)
		if r := t.LastExecutionResult; r != nil && !r.EndTimeUtc.IsZero() {
			style := styleMuted
			if !strings.EqualFold(r.Status, "Completed") {
				style = styleErr
			}
			last = col(fmt.Sprintf("%s ago · %s", relTime(r.EndTimeUtc), strings.ToLower(r.Status)), wLast, style)
		}

		b.WriteString(renderRow(i == p.cur.idx,
			col(t.Name, wName, styleText), state, bar, last,
			col(t.Category, wCat, styleFaint),
		))
		if i < hi-1 {
			b.WriteString("\n")
		}
	}
	return fillHeight(b.String(), h)
}
