package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NilssonMandola/jellytop/internal/jellyfin"
	tea "github.com/charmbracelet/bubbletea"
)

type librariesMsg struct {
	items []jellyfin.Library
	err   error
}

type librariesPane struct {
	client *jellyfin.Client
	items  []jellyfin.Library
	cur    cursor
	err    error
}

func newLibrariesPane(c *jellyfin.Client) *librariesPane { return &librariesPane{client: c} }

func (p *librariesPane) Title() string           { return "Libraries" }
func (p *librariesPane) Interval() time.Duration { return 30 * time.Second }
func (p *librariesPane) Err() error              { return p.err }

func (p *librariesPane) Load() tea.Cmd {
	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		items, err := c.Libraries(ctx)
		return librariesMsg{items: items, err: err}
	}
}

func (p *librariesPane) Summary() string {
	paths := 0
	for _, l := range p.items {
		paths += len(l.Locations)
	}
	return fmt.Sprintf("%d libraries · %d paths", len(p.items), paths)
}

func (p *librariesPane) Keys() []keyHint {
	return []keyHint{
		{"S", "scan all libraries"},
		{"r", "refresh"},
	}
}

func (p *librariesPane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case librariesMsg:
		p.err = msg.err
		if msg.err == nil {
			p.items = msg.items
			p.cur.move(0, len(p.items))
		}
		return nil, true

	case tea.KeyMsg:
		if msg.String() == "S" {
			c := p.client
			// Jellyfin exposes no per-library scan, so this is all-or-nothing.
			return confirm("Scan all libraries now?", false, func() tea.Cmd {
				return runAction("library scan started", func(ctx context.Context) error {
					return c.RefreshLibraries(ctx)
				})
			}), true
		}
	}
	return nil, false
}

func (p *librariesPane) MoveCursor(delta int) { p.cur.move(delta, len(p.items)) }
func (p *librariesPane) Home()                { p.cur.toTop() }
func (p *librariesPane) End()                 { p.cur.toEnd(len(p.items)) }

func (p *librariesPane) View(w, h int) string {
	if len(p.items) == 0 {
		return emptyState("No libraries configured.", w, h)
	}

	const (
		wName = 26
		wType = 14
		wRefr = 12
	)
	wPaths := w - (wName + wType + wRefr + 3)
	if wPaths < 12 {
		wPaths = 12
	}

	var b strings.Builder
	b.WriteString(renderRow(false,
		col("LIBRARY", wName, styleHeaderRow), col("TYPE", wType, styleHeaderRow),
		col("REFRESH", wRefr, styleHeaderRow), col("PATHS", wPaths, styleHeaderRow),
	))
	b.WriteString("\n")

	lo, hi := p.cur.window(len(p.items), h-1)
	for i := lo; i < hi; i++ {
		l := p.items[i]

		refresh := col("idle", wRefr, styleFaint)
		if l.RefreshStatus != "" && !strings.EqualFold(l.RefreshStatus, "Idle") {
			refresh = col(fmt.Sprintf("%.0f%%", l.RefreshProgress), wRefr, styleOK)
		}
		kind := l.CollectionType
		if kind == "" {
			kind = "mixed"
		}

		b.WriteString(renderRow(i == p.cur.idx,
			col(l.Name, wName, styleText), col(kind, wType, styleMuted), refresh,
			col(strings.Join(l.Locations, ", "), wPaths, styleFaint),
		))
		if i < hi-1 {
			b.WriteString("\n")
		}
	}
	return fillHeight(b.String(), h)
}
