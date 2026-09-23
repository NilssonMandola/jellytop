package jellyfin

import (
	"fmt"
	"time"
)

// Ticks are Jellyfin's unit of time: 100-nanosecond intervals.
const ticksPerSecond = 10_000_000

// TicksToDuration converts a Jellyfin tick count to a Duration.
func TicksToDuration(ticks int64) time.Duration {
	return time.Duration(ticks/ticksPerSecond) * time.Second
}

type SystemInfo struct {
	ServerName      string `json:"ServerName"`
	Version         string `json:"Version"`
	ID              string `json:"Id"`
	OperatingSystem string `json:"OperatingSystem"`
	StartupWizard   bool   `json:"StartupWizardCompleted"`
}

type PlayerState struct {
	PositionTicks int64  `json:"PositionTicks"`
	CanSeek       bool   `json:"CanSeek"`
	IsPaused      bool   `json:"IsPaused"`
	IsMuted       bool   `json:"IsMuted"`
	PlayMethod    string `json:"PlayMethod"`
	MediaSourceID string `json:"MediaSourceId"`
}

type TranscodingInfo struct {
	AudioCodec       string   `json:"AudioCodec"`
	VideoCodec       string   `json:"VideoCodec"`
	Container        string   `json:"Container"`
	IsVideoDirect    bool     `json:"IsVideoDirect"`
	IsAudioDirect    bool     `json:"IsAudioDirect"`
	Bitrate          int      `json:"Bitrate"`
	Framerate        float64  `json:"Framerate"`
	Width            int      `json:"Width"`
	Height           int      `json:"Height"`
	AudioChannels    int      `json:"AudioChannels"`
	HardwareAccel    string   `json:"HardwareAccelerationType"`
	TranscodeReasons []string `json:"TranscodeReasons"`
}

type NowPlayingItem struct {
	Name             string `json:"Name"`
	Type             string `json:"Type"`
	SeriesName       string `json:"SeriesName"`
	ParentIndexNum   *int   `json:"ParentIndexNumber"`
	IndexNumber      *int   `json:"IndexNumber"`
	RunTimeTicks     int64  `json:"RunTimeTicks"`
	ProductionYear   *int   `json:"ProductionYear"`
	ID               string `json:"Id"`
	AlbumArtist      string `json:"AlbumArtist"`
	Album            string `json:"Album"`
	MediaTypeRawName string `json:"MediaType"`
}

// Title renders the item the way a human would say it: "Severance S01E03 — Ep name"
// for episodes, "Dune: Part Two (2024)" for films, "Artist — Track" for music.
func (n *NowPlayingItem) Title() string {
	if n == nil {
		return ""
	}
	switch n.Type {
	case "Episode":
		s := n.SeriesName
		if n.ParentIndexNum != nil && n.IndexNumber != nil {
			s += fmt.Sprintf(" S%02dE%02d", *n.ParentIndexNum, *n.IndexNumber)
		}
		if n.Name != "" {
			s += " — " + n.Name
		}
		return s
	case "Audio":
		if n.AlbumArtist != "" {
			return n.AlbumArtist + " — " + n.Name
		}
		return n.Name
	default:
		if n.ProductionYear != nil {
			return fmt.Sprintf("%s (%d)", n.Name, *n.ProductionYear)
		}
		return n.Name
	}
}

type Session struct {
	ID                  string           `json:"Id"`
	UserID              string           `json:"UserId"`
	UserName            string           `json:"UserName"`
	Client              string           `json:"Client"`
	DeviceName          string           `json:"DeviceName"`
	DeviceID            string           `json:"DeviceId"`
	DeviceType          string           `json:"DeviceType"`
	ApplicationVersion  string           `json:"ApplicationVersion"`
	RemoteEndPoint      string           `json:"RemoteEndPoint"`
	IsActive            bool             `json:"IsActive"`
	SupportsMediaCtl    bool             `json:"SupportsMediaControl"`
	SupportsRemoteCtl   bool             `json:"SupportsRemoteControl"`
	LastActivityDate    time.Time        `json:"LastActivityDate"`
	LastPlaybackCheckIn time.Time        `json:"LastPlaybackCheckIn"`
	NowPlayingItem      *NowPlayingItem  `json:"NowPlayingItem"`
	PlayState           *PlayerState     `json:"PlayState"`
	TranscodingInfo     *TranscodingInfo `json:"TranscodingInfo"`
}

// Playing reports whether this session has media loaded.
func (s Session) Playing() bool { return s.NowPlayingItem != nil }

// Progress returns playback position as a fraction in [0,1].
func (s Session) Progress() float64 {
	if s.NowPlayingItem == nil || s.PlayState == nil || s.NowPlayingItem.RunTimeTicks <= 0 {
		return 0
	}
	p := float64(s.PlayState.PositionTicks) / float64(s.NowPlayingItem.RunTimeTicks)
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// TranscodeBitrate returns the outbound bitrate in bits per second for a
// transcoding session. Direct-play sessions carry no bitrate here; their
// figure comes from the item's media source (see MediaSourceBitrates).
func (s Session) TranscodeBitrate() (int64, bool) {
	if s.TranscodingInfo == nil || s.TranscodingInfo.Bitrate <= 0 {
		return 0, false
	}
	return int64(s.TranscodingInfo.Bitrate), true
}

// MediaSourceID identifies which media source a session is playing, used to
// look its bitrate up.
func (s Session) MediaSourceID() string {
	if s.PlayState == nil {
		return ""
	}
	return s.PlayState.MediaSourceID
}

// StreamKind summarises how the media is reaching the client.
func (s Session) StreamKind() string {
	if s.TranscodingInfo == nil {
		if s.PlayState != nil && s.PlayState.PlayMethod != "" {
			return s.PlayState.PlayMethod
		}
		return "Direct Play"
	}
	t := s.TranscodingInfo
	switch {
	case t.IsVideoDirect && t.IsAudioDirect:
		return "Remux"
	case t.IsVideoDirect:
		return "Transcode (audio)"
	case t.IsAudioDirect:
		return "Transcode (video)"
	default:
		return "Transcode"
	}
}

type ActivityEntry struct {
	ID            int64     `json:"Id"`
	Name          string    `json:"Name"`
	Overview      string    `json:"Overview"`
	ShortOverview string    `json:"ShortOverview"`
	Type          string    `json:"Type"`
	ItemID        string    `json:"ItemId"`
	Date          time.Time `json:"Date"`
	UserID        string    `json:"UserId"`
	Severity      string    `json:"Severity"`
}

type activityResult struct {
	Items            []ActivityEntry `json:"Items"`
	TotalRecordCount int             `json:"TotalRecordCount"`
}

type UserPolicy struct {
	IsAdministrator          bool     `json:"IsAdministrator"`
	IsDisabled               bool     `json:"IsDisabled"`
	IsHidden                 bool     `json:"IsHidden"`
	EnableRemoteAccess       bool     `json:"EnableRemoteAccess"`
	EnableContentDownloading bool     `json:"EnableContentDownloading"`
	EnabledFolders           []string `json:"EnabledFolders"`
}

type User struct {
	Name             string      `json:"Name"`
	ID               string      `json:"Id"`
	HasPassword      bool        `json:"HasPassword"`
	LastLoginDate    *time.Time  `json:"LastLoginDate"`
	LastActivityDate *time.Time  `json:"LastActivityDate"`
	Policy           *UserPolicy `json:"Policy"`
}

type Library struct {
	Name            string   `json:"Name"`
	Locations       []string `json:"Locations"`
	CollectionType  string   `json:"CollectionType"`
	ItemID          string   `json:"ItemId"`
	RefreshProgress float64  `json:"RefreshProgress"`
	RefreshStatus   string   `json:"RefreshStatus"`
}

type TaskTrigger struct {
	Type           string `json:"Type"`
	TimeOfDayTicks *int64 `json:"TimeOfDayTicks"`
	IntervalTicks  *int64 `json:"IntervalTicks"`
	DayOfWeek      string `json:"DayOfWeek"`
}

type TaskResult struct {
	StartTimeUtc time.Time `json:"StartTimeUtc"`
	EndTimeUtc   time.Time `json:"EndTimeUtc"`
	Status       string    `json:"Status"`
	Name         string    `json:"Name"`
	Key          string    `json:"Key"`
	ErrorMessage string    `json:"ErrorMessage"`
}

type Task struct {
	Name                string        `json:"Name"`
	State               string        `json:"State"`
	CurrentProgressPct  *float64      `json:"CurrentProgressPercentage"`
	ID                  string        `json:"Id"`
	LastExecutionResult *TaskResult   `json:"LastExecutionResult"`
	Triggers            []TaskTrigger `json:"Triggers"`
	Description         string        `json:"Description"`
	Category            string        `json:"Category"`
	IsHidden            bool          `json:"IsHidden"`
	Key                 string        `json:"Key"`
}

// Running reports whether the task is executing right now.
func (t Task) Running() bool { return t.State == "Running" || t.State == "Cancelling" }
