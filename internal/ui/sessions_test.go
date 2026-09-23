package ui

import (
	"testing"
	"time"

	"github.com/NilssonMandola/jellytop/internal/jellyfin"
	"github.com/charmbracelet/lipgloss"
)

func playing(sourceID string) jellyfin.Session {
	return jellyfin.Session{
		NowPlayingItem: &jellyfin.NowPlayingItem{ID: "item1", Name: "x", RunTimeTicks: 100},
		PlayState:      &jellyfin.PlayerState{MediaSourceID: sourceID},
	}
}

func TestBitrateCacheExpires(t *testing.T) {
	p := newSessionsPane(nil)
	p.items = []jellyfin.Session{playing("src1")}

	// Nothing cached: a lookup should be scheduled.
	if p.fetchBitrates() == nil {
		t.Fatal("expected a fetch for an uncached media source")
	}

	// Fresh entry: no refetch, and the value is reported.
	p.bitrates["src1"] = cachedBitrate{bps: 13_793_791, fetchedAt: time.Now()}
	if p.fetchBitrates() != nil {
		t.Error("a fresh cache entry should not trigger a refetch")
	}
	if r, ok := p.bitrate(p.items[0]); !ok || r.bps != 13_793_791 || !r.estimated {
		t.Errorf("bitrate = %+v, %v; want {13793791 true}, true", r, ok)
	}

	// Stale entry: refetch, but keep showing the old figure meanwhile.
	p.bitrates["src1"] = cachedBitrate{bps: 13_793_791, fetchedAt: time.Now().Add(-bitrateTTL - time.Second)}
	if p.fetchBitrates() == nil {
		t.Error("a stale cache entry should trigger a refetch: a replaced file keeps its media source ID")
	}
	if r, ok := p.bitrate(p.items[0]); !ok || r.bps != 13_793_791 {
		t.Errorf("stale value should still render, got %+v, %v", r, ok)
	}
}

func TestTranscodeBitrateSkipsTheCache(t *testing.T) {
	p := newSessionsPane(nil)
	s := playing("src1")
	s.TranscodingInfo = &jellyfin.TranscodingInfo{Bitrate: 4_000_000}
	p.items = []jellyfin.Session{s}

	if p.fetchBitrates() != nil {
		t.Error("a transcoding session reports its own bitrate; no lookup needed")
	}
	if r, ok := p.bitrate(s); !ok || r.bps != 4_000_000 {
		t.Errorf("bitrate = %+v, %v; want 4000000, true", r, ok)
	}
	if r, _ := p.bitrate(s); r.estimated {
		t.Error("a transcode reports a live outbound rate; it must not be tagged (avg)")
	}
}

func TestIdleSessionHasNoBitrate(t *testing.T) {
	p := newSessionsPane(nil)
	if _, ok := p.bitrate(jellyfin.Session{}); ok {
		t.Error("an idle session should report no bitrate")
	}
}

func TestRateLabel(t *testing.T) {
	if got := (sessionRate{bps: 13_793_791, estimated: true}).label(); got != "13.8 Mbps (avg)" {
		t.Errorf("label() = %q, want %q", got, "13.8 Mbps (avg)")
	}
	if got := (sessionRate{bps: 4_000_000}).label(); got != "4.0 Mbps" {
		t.Errorf("label() = %q, want %q", got, "4.0 Mbps")
	}
}

func TestRateCellWidthIsStable(t *testing.T) {
	for _, r := range []sessionRate{
		{bps: 13_793_791, estimated: true},
		{bps: 4_000_000},
	} {
		c := rateCell(r, 16)
		if c.raw != "" && lipgloss.Width(c.raw) != 16 {
			t.Errorf("rateCell(%+v).raw width = %d, want 16", r, lipgloss.Width(c.raw))
		}
		if lipgloss.Width(renderRow(false, c)) != 16 {
			t.Errorf("rendered rate cell for %+v is not 16 cells wide", r)
		}
	}
}
