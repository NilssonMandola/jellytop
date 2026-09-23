package jellyfin

import (
	"testing"
	"time"
)

func intp(i int) *int { return &i }

func TestNowPlayingItemTitle(t *testing.T) {
	tests := []struct {
		name string
		item NowPlayingItem
		want string
	}{
		{
			name: "episode with season and number",
			item: NowPlayingItem{Type: "Episode", SeriesName: "Severance", Name: "In Perpetuity",
				ParentIndexNum: intp(1), IndexNumber: intp(3)},
			want: "Severance S01E03 — In Perpetuity",
		},
		{
			name: "episode missing numbering falls back to series and name",
			item: NowPlayingItem{Type: "Episode", SeriesName: "Severance", Name: "In Perpetuity"},
			want: "Severance — In Perpetuity",
		},
		{
			name: "movie gains its year",
			item: NowPlayingItem{Type: "Movie", Name: "Dune: Part Two", ProductionYear: intp(2024)},
			want: "Dune: Part Two (2024)",
		},
		{
			name: "movie without a year is left alone",
			item: NowPlayingItem{Type: "Movie", Name: "Dune: Part Two"},
			want: "Dune: Part Two",
		},
		{
			name: "audio is credited to the album artist",
			item: NowPlayingItem{Type: "Audio", Name: "Idioteque", AlbumArtist: "Radiohead"},
			want: "Radiohead — Idioteque",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.Title(); got != tt.want {
				t.Errorf("Title() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNilItemTitleIsEmpty(t *testing.T) {
	var n *NowPlayingItem
	if got := n.Title(); got != "" {
		t.Errorf("nil Title() = %q, want empty", got)
	}
}

func TestSessionProgress(t *testing.T) {
	tests := []struct {
		name string
		s    Session
		want float64
	}{
		{"no item", Session{}, 0},
		{"no playstate", Session{NowPlayingItem: &NowPlayingItem{RunTimeTicks: 100}}, 0},
		{"zero runtime avoids divide by zero", Session{
			NowPlayingItem: &NowPlayingItem{RunTimeTicks: 0},
			PlayState:      &PlayerState{PositionTicks: 50},
		}, 0},
		{"halfway", Session{
			NowPlayingItem: &NowPlayingItem{RunTimeTicks: 100},
			PlayState:      &PlayerState{PositionTicks: 50},
		}, 0.5},
		{"position past runtime clamps to 1", Session{
			NowPlayingItem: &NowPlayingItem{RunTimeTicks: 100},
			PlayState:      &PlayerState{PositionTicks: 250},
		}, 1},
		{"negative position clamps to 0", Session{
			NowPlayingItem: &NowPlayingItem{RunTimeTicks: 100},
			PlayState:      &PlayerState{PositionTicks: -5},
		}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Progress(); got != tt.want {
				t.Errorf("Progress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionStreamKind(t *testing.T) {
	tests := []struct {
		name string
		s    Session
		want string
	}{
		{"no transcode info falls back to play method",
			Session{PlayState: &PlayerState{PlayMethod: "DirectStream"}}, "DirectStream"},
		{"no info at all reads as direct play", Session{}, "Direct Play"},
		{"both streams direct is a remux",
			Session{TranscodingInfo: &TranscodingInfo{IsVideoDirect: true, IsAudioDirect: true}}, "Remux"},
		{"video direct means audio is being transcoded",
			Session{TranscodingInfo: &TranscodingInfo{IsVideoDirect: true}}, "Transcode (audio)"},
		{"audio direct means video is being transcoded",
			Session{TranscodingInfo: &TranscodingInfo{IsAudioDirect: true}}, "Transcode (video)"},
		{"neither direct is a full transcode",
			Session{TranscodingInfo: &TranscodingInfo{}}, "Transcode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.StreamKind(); got != tt.want {
				t.Errorf("StreamKind() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTicksToDuration(t *testing.T) {
	if got := TicksToDuration(45 * ticksPerSecond); got != 45*time.Second {
		t.Errorf("TicksToDuration = %v, want 45s", got)
	}
	if got := TicksToDuration(0); got != 0 {
		t.Errorf("TicksToDuration(0) = %v, want 0", got)
	}
}

func TestTaskRunning(t *testing.T) {
	for state, want := range map[string]bool{
		"Running": true, "Cancelling": true, "Idle": false, "": false,
	} {
		if got := (Task{State: state}).Running(); got != want {
			t.Errorf("Task{State:%q}.Running() = %v, want %v", state, got, want)
		}
	}
}

func TestTranscodeBitrate(t *testing.T) {
	if _, ok := (Session{}).TranscodeBitrate(); ok {
		t.Error("a session with no transcode info should report no bitrate")
	}
	if _, ok := (Session{TranscodingInfo: &TranscodingInfo{Bitrate: 0}}).TranscodeBitrate(); ok {
		t.Error("a zero bitrate should count as unknown, not as 0 bps")
	}
	got, ok := (Session{TranscodingInfo: &TranscodingInfo{Bitrate: 4_000_000}}).TranscodeBitrate()
	if !ok || got != 4_000_000 {
		t.Errorf("TranscodeBitrate() = %d, %v; want 4000000, true", got, ok)
	}
}

func TestMediaSourceID(t *testing.T) {
	if got := (Session{}).MediaSourceID(); got != "" {
		t.Errorf("no play state should yield an empty source ID, got %q", got)
	}
	s := Session{PlayState: &PlayerState{MediaSourceID: "abc"}}
	if got := s.MediaSourceID(); got != "abc" {
		t.Errorf("MediaSourceID() = %q, want abc", got)
	}
}
