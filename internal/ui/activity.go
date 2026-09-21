package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/NilssonMandola/jellytop/internal/jellyfin"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const activityPageSize = 100

type activityMsg struct {
	items      []jellyfin.ActivityEntry
	total      int
	startIndex int
	err        error
}

type activityPane struct {
	client     *jellyfin.Client
	items      []jellyfin.ActivityEntry
	total      int
	startIndex int
	filter     string
	cur        cursor
	err        error
	detail     bool
}

func newActivityPane(c *jellyfin.Client) *activityPane { return &activityPane{client: c} }

func (p *activityPane) Title() string           { return "Activity" }
func (p *activityPane) Interval() time.Duration { return 15 * time.Second }
func (p *activityPane) Err() error              { return p.err }

func (p *activityPane) Load() tea.Cmd {
	c, start := p.client, p.startIndex
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		items, total, err := c.Activity(ctx, start, activityPageSize)
		return activityMsg{items: items, total: total, startIndex: start, err: err}
	}
}

// visible applies the current filter to the loaded page.
func (p *activityPane) visible() []jellyfin.ActivityEntry {
	if p.filter == "" {
		return p.items
	}
	needle := strings.ToLower(p.filter)
	out := make([]jellyfin.ActivityEntry, 0, len(p.items))
	for _, e := range p.items {
		hay := strings.ToLower(e.Name + " " + e.ShortOverview + " " + e.Overview + " " + e.Type + " " + e.Severity)
		if strings.Contains(hay, needle) {
			out = append(out, e)
		}
	}
	return out
}

func (p *activityPane) Summary() string {
	page := p.startIndex/activityPageSize + 1
	pages := (p.total + activityPageSize - 1) / activityPageSize
	if pages == 0 {
		pages = 1
	}
	s := fmt.Sprintf("%d entries · page %d/%d", p.total, page, pages)
	if p.filter != "" {
		s = fmt.Sprintf("filter %q · %d on this page · %s", p.filter, len(p.visible()), s)
	}
	return s
}

func (p *activityPane) Keys() []keyHint {
	if p.detail {
		return []keyHint{{"esc/enter", "back"}}
	}
	return []keyHint{
		{"enter", "details"},
		{"/", "filter"},
		{"n/p", "page"},
		{"r", "refresh"},
	}
}

func (p *activityPane) selected() (jellyfin.ActivityEntry, bool) {
	v := p.visible()
	if p.cur.idx < 0 || p.cur.idx >= len(v) {
		return jellyfin.ActivityEntry{}, false
	}
	return v[p.cur.idx], true
}

func (p *activityPane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case activityMsg:
		p.err = msg.err
		if msg.err == nil {
			p.items, p.total, p.startIndex = msg.items, msg.total, msg.startIndex
			p.cur.move(0, len(p.visible()))
		}
		return nil, true

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if _, ok := p.selected(); ok {
				p.detail = !p.detail
			}
			return nil, true
		case "esc":
			if p.detail {
				p.detail = false
				return nil, true
			}
			if p.filter != "" {
				p.filter = ""
				p.cur.toTop()
				return nil, true
			}
		case "/":
			return prompt("Filter activity", "substring match…", func(text string) tea.Cmd {
				p.filter = strings.TrimSpace(text)
				p.cur.toTop()
				return nil
			}), true
		case "n":
			if p.startIndex+activityPageSize < p.total {
				p.startIndex += activityPageSize
				p.cur.toTop()
				return p.Load(), true
			}
			return nil, true
		case "p":
			if p.startIndex > 0 {
				p.startIndex -= activityPageSize
				if p.startIndex < 0 {
					p.startIndex = 0
				}
				p.cur.toTop()
				return p.Load(), true
			}
			return nil, true
		}
	}
	return nil, false
}

func (p *activityPane) MoveCursor(delta int) { p.cur.move(delta, len(p.visible())) }
func (p *activityPane) Home()                { p.cur.toTop() }
func (p *activityPane) End()                 { p.cur.toEnd(len(p.visible())) }

func severityStyle(sev string) (string, lipgloss.Style) {
	switch strings.ToLower(sev) {
	case "error", "critical", "fatal":
		return "●", styleErr
	case "warning", "warn":
		return "●", styleWarn
	default:
		return "·", styleFaint
	}
}

func (p *activityPane) View(w, h int) string {
	if p.detail {
		return fillHeight(p.viewDetail(w), h)
	}
	items := p.visible()
	if len(items) == 0 {
		if p.filter != "" {
			return emptyState(fmt.Sprintf("Nothing on this page matches %q — esc clears the filter.", p.filter), w, h)
		}
		return emptyState("Activity log is empty.", w, h)
	}

	// Event names are long and detail is usually short ("VideoPlaybackStopped",
	// "IP address: …"), so the event column takes the slack.
	const (
		wMark   = 1
		wWhen   = 16
		wDetail = 30
	)
	wName := w - (wMark + wWhen + wDetail + 3)
	if wName < 20 {
		wName = 20
	}

	var b strings.Builder
	b.WriteString(renderRow(false,
		col("", wMark, styleHeaderRow), col("WHEN", wWhen, styleHeaderRow),
		col("EVENT", wName, styleHeaderRow), col("DETAIL", wDetail, styleHeaderRow),
	))
	b.WriteString("\n")

	lo, hi := p.cur.window(len(items), h-1)
	for i := lo; i < hi; i++ {
		e := items[i]
		mark, ms := severityStyle(e.Severity)
		when := e.Date.Local().Format("Jan 02 15:04")
		detail := e.ShortOverview
		if detail == "" {
			detail = e.Overview
		}
		if detail == "" {
			detail = e.Type
		}
		b.WriteString(renderRow(i == p.cur.idx,
			col(mark, wMark, ms),
			col(when, wWhen, styleMuted),
			col(e.Name, wName, styleText),
			col(strings.ReplaceAll(detail, "\n", " "), wDetail, styleMuted),
		))
		if i < hi-1 {
			b.WriteString("\n")
		}
	}
	return fillHeight(b.String(), h)
}

func (p *activityPane) viewDetail(w int) string {
	e, ok := p.selected()
	if !ok {
		return ""
	}
	var b strings.Builder
	line := func(k, v string) {
		if v == "" {
			v = "—"
		}
		b.WriteString(styleMuted.Render(pad(k, 18)) + styleText.Render(fit(v, w-19)) + "\n")
	}
	b.WriteString(styleTitle.Render(e.Name) + "\n\n")
	line("When", e.Date.Local().Format("2006-01-02 15:04:05 MST")+"  ("+relTime(e.Date)+" ago)")
	line("Type", e.Type)
	line("Severity", e.Severity)
	line("User ID", e.UserID)
	line("Item ID", e.ItemID)
	line("Short overview", e.ShortOverview)
	if e.Overview != "" {
		b.WriteString("\n" + styleMuted.Render("Overview") + "\n")
		for _, l := range wrap(e.Overview, w-2) {
			b.WriteString(styleText.Render(l) + "\n")
		}
	}
	return b.String()
}
