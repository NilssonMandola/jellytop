package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

// cursor tracks selection and scroll offset for a list of n rows shown in a
// viewport of h rows.
type cursor struct {
	idx int
	off int
}

func (c *cursor) move(delta, n int) {
	if n == 0 {
		c.idx, c.off = 0, 0
		return
	}
	c.idx += delta
	if c.idx < 0 {
		c.idx = 0
	}
	if c.idx >= n {
		c.idx = n - 1
	}
}

func (c *cursor) toTop()      { c.idx = 0 }
func (c *cursor) toEnd(n int) { c.move(n, n) }

// window returns the slice bounds to render, scrolling to keep idx visible.
func (c *cursor) window(n, h int) (lo, hi int) {
	if h < 1 {
		h = 1
	}
	if n <= h {
		c.off = 0
		return 0, n
	}
	if c.idx < c.off {
		c.off = c.idx
	}
	if c.idx >= c.off+h {
		c.off = c.idx - h + 1
	}
	if c.off > n-h {
		c.off = n - h
	}
	if c.off < 0 {
		c.off = 0
	}
	return c.off, c.off + h
}

// pad trims or space-pads s to exactly w display cells.
func pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.FillRight(runewidth.Truncate(s, w, "…"), w)
}

// fit truncates plain text to w cells without padding.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return runewidth.Truncate(s, w, "…")
}

// fitStyled truncates a string that already contains ANSI styling. Plain
// width maths would count escape sequences as visible cells and cut the line
// far too short.
func fitStyled(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// progressBar renders a proportional bar w cells wide, returning both a plain
// and a two-tone styled form. The plain form is what selected rows use, so the
// selection background stays unbroken.
func progressBar(frac float64, w int, style lipgloss.Style) (plain, styled string) {
	if w <= 0 {
		return "", ""
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac*float64(w) + 0.5)
	full, empty := strings.Repeat("█", filled), strings.Repeat("░", w-filled)
	return full + empty, style.Render(full) + styleFaint.Render(empty)
}

// cell is one column of a table row. text is the plain content used for width
// maths and for selected rows; raw, when set, is a pre-styled rendering used
// for unselected rows (a two-tone progress bar, say).
type cell struct {
	text  string
	w     int
	style lipgloss.Style
	raw   string
}

// col builds a plain styled cell.
func col(text string, w int, style lipgloss.Style) cell {
	return cell{text: text, w: w, style: style}
}

// renderRow lays out cells separated by a single space. A selected row is
// painted entirely in the selection style, separators included, so the
// highlight reads as one continuous band.
func renderRow(selected bool, cells ...cell) string {
	parts := make([]string, 0, len(cells))
	for _, c := range cells {
		switch {
		case selected:
			parts = append(parts, styleSelected.Render(pad(c.text, c.w)))
		case c.raw != "":
			parts = append(parts, c.raw)
		default:
			parts = append(parts, c.style.Render(pad(c.text, c.w)))
		}
	}
	sep := " "
	if selected {
		sep = styleSelected.Render(" ")
	}
	return strings.Join(parts, sep)
}

// hhmmss formats a duration as 1:02:03 or 2:03.
func hhmmss(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	h, m, s := total/3600, (total%3600)/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// bitrate renders bits per second as kbps, Mbps or Gbps. Jellyfin's figures
// are decimal (a 13793791 bps file is "13.8 Mbps"), so the divisor is 1000.
func bitrate(bps int64) string {
	switch {
	case bps <= 0:
		return "—"
	case bps < 1_000_000:
		return fmt.Sprintf("%d kbps", bps/1_000)
	case bps < 1_000_000_000:
		return fmt.Sprintf("%.1f Mbps", float64(bps)/1_000_000)
	default:
		return fmt.Sprintf("%.2f Gbps", float64(bps)/1_000_000_000)
	}
}

// relTime renders a timestamp as a compact age: 4s, 12m, 3h, 6d.
func relTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := time.Since(t)
	switch {
	case d < 0:
		return "now"
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// wrap breaks s into lines of at most w display cells, splitting on spaces.
func wrap(s string, w int) []string {
	if w < 8 {
		w = 8
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			switch {
			case line == "":
				line = word
			case runewidth.StringWidth(line)+1+runewidth.StringWidth(word) <= w:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return out
}

// emptyState renders a centred hint when a pane has no rows.
func emptyState(msg string, w, h int) string {
	if h < 1 {
		return ""
	}
	lines := make([]string, h)
	mid := h / 2
	for i := range lines {
		if i == mid {
			lines[i] = lipgloss.PlaceHorizontal(w, lipgloss.Center, styleFaint.Render(msg))
		} else {
			lines[i] = strings.Repeat(" ", 0)
		}
	}
	return strings.Join(lines, "\n")
}

// fillHeight pads a block of lines to exactly h lines so panes don't jitter.
func fillHeight(s string, h int) string {
	if h < 0 {
		h = 0
	}
	lines := strings.Split(s, "\n")
	if s == "" {
		lines = nil
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines[:h], "\n")
}
