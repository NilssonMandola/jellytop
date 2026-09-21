package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func TestPadIsExactWidth(t *testing.T) {
	for _, tt := range []struct {
		in string
		w  int
	}{
		{"short", 12}, {"exactly-ten", 11}, {"far too long to fit here", 8}, {"", 5},
	} {
		if got := lipgloss.Width(pad(tt.in, tt.w)); got != tt.w {
			t.Errorf("pad(%q, %d) width = %d, want %d", tt.in, tt.w, got, tt.w)
		}
	}
	if pad("anything", 0) != "" {
		t.Error("pad to zero width should be empty")
	}
}

func TestRenderRowWidthIsStable(t *testing.T) {
	cells := []cell{col("user", 10, styleText), col("title", 20, styleMuted), col("x", 5, styleFaint)}
	// 3 cells plus 2 single-space separators.
	want := 10 + 20 + 5 + 2
	for _, selected := range []bool{false, true} {
		if got := lipgloss.Width(renderRow(selected, cells...)); got != want {
			t.Errorf("renderRow(selected=%v) width = %d, want %d", selected, got, want)
		}
	}
}

func TestRenderRowUsesRawWhenNotSelected(t *testing.T) {
	c := cell{text: "████░░░░", w: 8, style: styleOK, raw: "RAWMARKER"}
	if !strings.Contains(renderRow(false, c), "RAWMARKER") {
		t.Error("unselected row should use the pre-styled raw form")
	}
	if strings.Contains(renderRow(true, c), "RAWMARKER") {
		t.Error("selected row must use plain text so the highlight stays unbroken")
	}
}

func TestProgressBar(t *testing.T) {
	for _, tt := range []struct {
		frac float64
		want string
	}{
		{0, "░░░░░░░░░░"},
		{0.5, "█████░░░░░"},
		{1, "██████████"},
		{-1, "░░░░░░░░░░"}, // clamped
		{5, "██████████"},  // clamped
	} {
		plain, _ := progressBar(tt.frac, 10, styleOK)
		if plain != tt.want {
			t.Errorf("progressBar(%v) = %q, want %q", tt.frac, plain, tt.want)
		}
	}
	if plain, styled := progressBar(0.5, 0, styleOK); plain != "" || styled != "" {
		t.Error("zero-width bar should render nothing")
	}
}

func TestFitStyledCountsVisibleCells(t *testing.T) {
	// The styled string is only 9 cells wide but far longer in bytes; naive
	// truncation would cut it to nothing.
	s := styleOK.Render("hello") + styleErr.Render("1234")
	if got := lipgloss.Width(fitStyled(s, 20)); got != 9 {
		t.Errorf("fitStyled left %d cells, want the original 9", got)
	}
	if got := lipgloss.Width(fitStyled(s, 4)); got > 4 {
		t.Errorf("fitStyled(4) width = %d, want <= 4", got)
	}
}

func TestCursorWindowKeepsSelectionVisible(t *testing.T) {
	var c cursor
	c.idx = 25
	lo, hi := c.window(100, 10)
	if c.idx < lo || c.idx >= hi {
		t.Fatalf("cursor %d outside window [%d,%d)", c.idx, lo, hi)
	}
	if hi-lo != 10 {
		t.Errorf("window height = %d, want 10", hi-lo)
	}

	// Shorter list than the viewport shows everything from the top.
	c = cursor{}
	if lo, hi := c.window(3, 10); lo != 0 || hi != 3 {
		t.Errorf("window(3,10) = [%d,%d), want [0,3)", lo, hi)
	}

	// The window never scrolls past the end of the list.
	c = cursor{idx: 99}
	lo, hi = c.window(100, 10)
	if hi != 100 || lo != 90 {
		t.Errorf("window at end = [%d,%d), want [90,100)", lo, hi)
	}
}

func TestCursorMoveClamps(t *testing.T) {
	var c cursor
	c.move(-5, 10)
	if c.idx != 0 {
		t.Errorf("idx = %d, want 0", c.idx)
	}
	c.move(50, 10)
	if c.idx != 9 {
		t.Errorf("idx = %d, want 9", c.idx)
	}
	c.move(3, 0) // empty list
	if c.idx != 0 {
		t.Errorf("idx = %d on empty list, want 0", c.idx)
	}
}

func TestFillHeightIsExact(t *testing.T) {
	for _, tt := range []struct {
		in string
		h  int
	}{
		{"a\nb", 5}, {"a\nb\nc\nd", 2}, {"", 3},
	} {
		got := len(strings.Split(fillHeight(tt.in, tt.h), "\n"))
		if got != tt.h {
			t.Errorf("fillHeight(%q, %d) produced %d lines", tt.in, tt.h, got)
		}
	}
}

func TestHHMMSS(t *testing.T) {
	for in, want := range map[time.Duration]string{
		0:                  "0:00",
		45 * time.Second:   "0:45",
		125 * time.Second:  "2:05",
		3725 * time.Second: "1:02:05",
		-5 * time.Second:   "0:00",
	} {
		if got := hhmmss(in); got != want {
			t.Errorf("hhmmss(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestRelTime(t *testing.T) {
	now := time.Now()
	for _, tt := range []struct {
		in   time.Time
		want string
	}{
		{time.Time{}, "—"},
		{now.Add(-30 * time.Second), "30s"},
		{now.Add(-5 * time.Minute), "5m"},
		{now.Add(-3 * time.Hour), "3h"},
		{now.Add(-48 * time.Hour), "2d"},
		{now.Add(time.Hour), "now"},
	} {
		if got := relTime(tt.in); got != tt.want {
			t.Errorf("relTime(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestWrapRespectsWidth(t *testing.T) {
	lines := wrap("the quick brown fox jumps over the lazy dog", 12)
	for _, l := range lines {
		if lipgloss.Width(l) > 12 {
			t.Errorf("line %q exceeds width 12", l)
		}
	}
	if len(lines) < 2 {
		t.Error("expected the text to wrap onto several lines")
	}
}
