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

type sessionsMsg struct {
	items []jellyfin.Session
	err   error
}

// bitratesMsg carries media-source bitrates looked up for direct-play sessions.
type bitratesMsg struct {
	rates map[string]int64
	err   error
}

type sessionsPane struct {
	client *jellyfin.Client
	items  []jellyfin.Session
	cur    cursor
	err    error
	detail bool
	// bitrates caches media source ID -> bits per second.
	bitrates map[string]cachedBitrate
}

// bitrateTTL bounds how long a cached figure is trusted. A media source ID is
// stable across a file being replaced — for a single-version movie it is just
// the item ID — so an *arr upgrading 1080p to 4K changes the bitrate without
// changing the key. Live streams drift for the same reason. Re-checking every
// few minutes costs one small request per playing title.
const bitrateTTL = 5 * time.Minute

type cachedBitrate struct {
	bps       int64
	fetchedAt time.Time
}

func (c cachedBitrate) fresh() bool { return time.Since(c.fetchedAt) < bitrateTTL }

func newSessionsPane(c *jellyfin.Client) *sessionsPane {
	return &sessionsPane{client: c, bitrates: map[string]cachedBitrate{}}
}

// sessionRate is a session's outbound bandwidth, and whether the figure is a
// live rate or an average over the whole file.
type sessionRate struct {
	bps int64
	// estimated marks a figure taken from the media source rather than
	// measured: the file's average bitrate, which is what a direct-play client
	// pulls in the steady state but not its instantaneous throughput.
	estimated bool
}

// label renders the rate, tagging averages so they are not mistaken for
// measurements.
func (r sessionRate) label() string {
	if r.estimated {
		return bitrate(r.bps) + " (avg)"
	}
	return bitrate(r.bps)
}

// bitrate returns the outbound bandwidth of a session. Transcoding sessions
// report a real outbound rate; direct-play sessions fall back to the cached
// media-source average.
func (p *sessionsPane) bitrate(s jellyfin.Session) (sessionRate, bool) {
	if !s.Playing() {
		return sessionRate{}, false
	}
	if bps, ok := s.TranscodeBitrate(); ok {
		return sessionRate{bps: bps}, true
	}
	// A stale entry is still shown while its replacement is in flight; a wrong
	// number for a few seconds beats the column flickering to "—".
	c, ok := p.bitrates[s.MediaSourceID()]
	return sessionRate{bps: c.bps, estimated: true}, ok
}

// fetchBitrates looks up any playing item whose bitrate is not cached yet.
func (p *sessionsPane) fetchBitrates() tea.Cmd {
	var ids []string
	seen := map[string]bool{}
	for _, s := range p.items {
		if !s.Playing() {
			continue
		}
		if _, ok := s.TranscodeBitrate(); ok {
			continue // transcodes report their own rate
		}
		if cached, ok := p.bitrates[s.MediaSourceID()]; ok && cached.fresh() {
			continue
		}
		if id := s.NowPlayingItem.ID; id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}

	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		rates, err := c.MediaSourceBitrates(ctx, ids)
		return bitratesMsg{rates: rates, err: err}
	}
}

func (p *sessionsPane) Title() string           { return "Sessions" }
func (p *sessionsPane) Interval() time.Duration { return 2 * time.Second }
func (p *sessionsPane) Err() error              { return p.err }

func (p *sessionsPane) Load() tea.Cmd {
	c := p.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		items, err := c.Sessions(ctx)
		return sessionsMsg{items: items, err: err}
	}
}

func (p *sessionsPane) Summary() string {
	streaming, total, anyEstimated := 0, int64(0), false
	for _, s := range p.items {
		if !s.Playing() {
			continue
		}
		streaming++
		if r, ok := p.bitrate(s); ok {
			total += r.bps
			anyEstimated = anyEstimated || r.estimated
		}
	}
	summary := fmt.Sprintf("%d sessions · %d streaming", len(p.items), streaming)
	if total > 0 {
		summary += " · " + bitrate(total) + " out"
		// One averaged component makes the whole total an average.
		if anyEstimated {
			summary += " (avg)"
		}
	}
	return summary
}

func (p *sessionsPane) Keys() []keyHint {
	if p.detail {
		return []keyHint{{"esc/enter", "back"}}
	}
	return []keyHint{
		{"enter", "details"},
		{"s", "stop playback"},
		{"m", "message"},
		{"r", "refresh"},
	}
}

func (p *sessionsPane) selected() (jellyfin.Session, bool) {
	if p.cur.idx < 0 || p.cur.idx >= len(p.items) {
		return jellyfin.Session{}, false
	}
	return p.items[p.cur.idx], true
}

func (p *sessionsPane) Handle(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case sessionsMsg:
		p.err = msg.err
		if msg.err == nil {
			p.items = msg.items
			p.cur.move(0, len(p.items))
			return p.fetchBitrates(), true
		}
		return nil, true

	case bitratesMsg:
		// A failed lookup is not worth surfacing: the column simply stays "—".
		now := time.Now()
		for id, bps := range msg.rates {
			p.bitrates[id] = cachedBitrate{bps: bps, fetchedAt: now}
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
		case "s":
			s, ok := p.selected()
			if !ok || !s.Playing() {
				return flash("nothing playing on that session", true), true
			}
			c := p.client
			return confirm(fmt.Sprintf("Stop %s on %s?", s.NowPlayingItem.Title(), s.DeviceName), true,
				func() tea.Cmd {
					return runAction(fmt.Sprintf("stopped playback on %s", s.DeviceName), func(ctx context.Context) error {
						return c.StopPlayback(ctx, s.ID)
					})
				}), true
		case "m":
			s, ok := p.selected()
			if !ok {
				return nil, true
			}
			c := p.client
			return prompt(fmt.Sprintf("Message to %s (%s)", s.UserName, s.DeviceName), "Type a message…",
				func(text string) tea.Cmd {
					if strings.TrimSpace(text) == "" {
						return nil
					}
					return runAction("message sent to "+s.DeviceName, func(ctx context.Context) error {
						return c.SendMessage(ctx, s.ID, "jellytop", text)
					})
				}), true
		}
	}
	return nil, false
}

func (p *sessionsPane) MoveCursor(delta int) { p.cur.move(delta, len(p.items)) }
func (p *sessionsPane) Home()                { p.cur.toTop() }
func (p *sessionsPane) End()                 { p.cur.toEnd(len(p.items)) }

func (p *sessionsPane) View(w, h int) string {
	if p.detail {
		return fillHeight(p.viewDetail(w), h)
	}
	if len(p.items) == 0 {
		return emptyState("No client sessions.", w, h)
	}

	// Fixed columns, with the title soaking up whatever is left.
	const (
		wMark   = 1
		wUser   = 12
		wDevice = 18
		wStream = 17
		wRate   = 16
		wBar    = 12
		wTime   = 15
	)
	wTitle := w - (wMark + wUser + wDevice + wStream + wRate + wBar + wTime + 7)
	if wTitle < 12 {
		wTitle = 12
	}

	var b strings.Builder
	b.WriteString(renderRow(false,
		col("", wMark, styleHeaderRow), col("USER", wUser, styleHeaderRow),
		col("NOW PLAYING", wTitle, styleHeaderRow), col("DEVICE", wDevice, styleHeaderRow),
		col("STREAM", wStream, styleHeaderRow), col("BITRATE", wRate, styleHeaderRow),
		col("", wBar, styleHeaderRow), col("", wTime, styleHeaderRow),
	))
	b.WriteString("\n")

	lo, hi := p.cur.window(len(p.items), h-1)
	for i := lo; i < hi; i++ {
		s := p.items[i]

		mark := col("○", wMark, styleFaint)
		title := col("idle", wTitle, styleFaint)
		stream := col("—", wStream, styleFaint)
		rate := col("—", wRate, styleFaint)
		bar := col("", wBar, styleFaint)
		when := col("seen "+relTime(s.LastActivityDate), wTime, styleFaint)

		if s.Playing() {
			paused := s.PlayState != nil && s.PlayState.IsPaused
			if paused {
				mark = col("⏸", wMark, styleWarn)
			} else {
				mark = col("▶", wMark, styleOK)
			}
			title = col(s.NowPlayingItem.Title(), wTitle, styleText)

			kind := s.StreamKind()
			if strings.HasPrefix(kind, "Transcode") {
				stream = col(kind, wStream, styleWarn)
			} else {
				stream = col(kind, wStream, styleOK)
			}

			if r, ok := p.bitrate(s); ok {
				rate = rateCell(r, wRate)
			}

			barStyle := styleOK
			if paused {
				barStyle = styleWarn
			}
			plain, styled := progressBar(s.Progress(), wBar, barStyle)
			bar = cell{text: plain, w: wBar, style: barStyle, raw: styled}

			if s.PlayState != nil {
				pos := jellyfin.TicksToDuration(s.PlayState.PositionTicks)
				total := jellyfin.TicksToDuration(s.NowPlayingItem.RunTimeTicks)
				when = col(fmt.Sprintf("%s/%s", hhmmss(pos), hhmmss(total)), wTime, styleMuted)
			}
		}

		b.WriteString(renderRow(i == p.cur.idx,
			mark, col(s.UserName, wUser, styleText), title,
			col(deviceLabel(s), wDevice, styleMuted), stream, rate, bar, when,
		))
		if i < hi-1 {
			b.WriteString("\n")
		}
	}
	return fillHeight(b.String(), h)
}

// rateCell renders a bitrate, dimming the "(avg)" tag so the number stays
// dominant while the caveat is still legible.
func rateCell(r sessionRate, w int) cell {
	plain := r.label()
	if !r.estimated {
		return col(plain, w, styleText)
	}

	num, tag := bitrate(r.bps), " (avg)"
	raw := styleText.Render(num) + styleFaint.Render(tag)
	if gap := w - lipgloss.Width(plain); gap > 0 {
		raw += strings.Repeat(" ", gap)
	} else if gap < 0 {
		raw = fitStyled(raw, w) // narrow terminal: fall back to truncation
	}
	return cell{text: plain, w: w, style: styleText, raw: raw}
}

func deviceLabel(s jellyfin.Session) string {
	if s.DeviceName != "" && s.Client != "" && s.DeviceName != s.Client {
		return s.Client + " · " + s.DeviceName
	}
	if s.DeviceName != "" {
		return s.DeviceName
	}
	return s.Client
}

func (p *sessionsPane) viewDetail(w int) string {
	s, ok := p.selected()
	if !ok {
		return ""
	}
	var b strings.Builder
	line := func(k, v string) {
		if v == "" {
			v = "—"
		}
		b.WriteString(styleMuted.Render(pad(k, 22)) + styleText.Render(fit(v, w-23)) + "\n")
	}

	b.WriteString(styleTitle.Render(s.UserName+" · "+deviceLabel(s)) + "\n\n")
	line("Session ID", s.ID)
	line("User", s.UserName)
	line("Client", fmt.Sprintf("%s %s", s.Client, s.ApplicationVersion))
	line("Device", s.DeviceName)
	line("Device type", s.DeviceType)
	line("Remote address", s.RemoteEndPoint)
	line("Last activity", relTime(s.LastActivityDate)+" ago")
	line("Remote control", yesNo(s.SupportsRemoteCtl))

	if s.Playing() {
		n := s.NowPlayingItem
		b.WriteString("\n" + styleTitle.Render("Now playing") + "\n\n")
		line("Title", n.Title())
		line("Type", n.Type)
		if s.PlayState != nil {
			pos := jellyfin.TicksToDuration(s.PlayState.PositionTicks)
			total := jellyfin.TicksToDuration(n.RunTimeTicks)
			line("Position", fmt.Sprintf("%s / %s (%.0f%%)", hhmmss(pos), hhmmss(total), s.Progress()*100))
			line("Paused", yesNo(s.PlayState.IsPaused))
			line("Play method", s.PlayState.PlayMethod)
		}
		if r, ok := p.bitrate(s); ok {
			if r.estimated {
				line("Bitrate", bitrate(r.bps)+"  (file average, not measured)")
			} else {
				line("Bitrate", bitrate(r.bps)+"  (outbound, live)")
			}
		}
		if t := s.TranscodingInfo; t != nil {
			b.WriteString("\n" + styleTitle.Render("Transcoding") + "\n\n")
			line("Video", fmt.Sprintf("%s  direct=%s", t.VideoCodec, yesNo(t.IsVideoDirect)))
			line("Audio", fmt.Sprintf("%s  %dch  direct=%s", t.AudioCodec, t.AudioChannels, yesNo(t.IsAudioDirect)))
			line("Container", t.Container)
			if t.Bitrate > 0 {
				line("Bitrate", fmt.Sprintf("%.1f Mbps", float64(t.Bitrate)/1_000_000))
			}
			if t.Width > 0 {
				line("Resolution", fmt.Sprintf("%dx%d", t.Width, t.Height))
			}
			line("Hardware accel", t.HardwareAccel)
			line("Reasons", strings.Join(t.TranscodeReasons, ", "))
		}
	}
	return b.String()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
