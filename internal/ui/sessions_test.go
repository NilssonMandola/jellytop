package ui

import (
	"testing"
	"time"

	"github.com/NilssonMandola/jellytop/internal/jellyfin"
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
	if bps, ok := p.bitrate(p.items[0]); !ok || bps != 13_793_791 {
		t.Errorf("bitrate = %d, %v; want 13793791, true", bps, ok)
	}

	// Stale entry: refetch, but keep showing the old figure meanwhile.
	p.bitrates["src1"] = cachedBitrate{bps: 13_793_791, fetchedAt: time.Now().Add(-bitrateTTL - time.Second)}
	if p.fetchBitrates() == nil {
		t.Error("a stale cache entry should trigger a refetch: a replaced file keeps its media source ID")
	}
	if bps, ok := p.bitrate(p.items[0]); !ok || bps != 13_793_791 {
		t.Errorf("stale value should still render, got %d, %v", bps, ok)
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
	if bps, ok := p.bitrate(s); !ok || bps != 4_000_000 {
		t.Errorf("bitrate = %d, %v; want 4000000, true", bps, ok)
	}
}

func TestIdleSessionHasNoBitrate(t *testing.T) {
	p := newSessionsPane(nil)
	if _, ok := p.bitrate(jellyfin.Session{}); ok {
		t.Error("an idle session should report no bitrate")
	}
}
