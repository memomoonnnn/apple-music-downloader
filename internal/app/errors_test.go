package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"amdl/internal/config"
	"amdl/internal/events"
	"amdl/internal/model"
	"amdl/internal/widevine-rip/runv5"
)

func TestTrackGetAlbumDataError(t *testing.T) {
	track := &model.Track{}
	err := track.GetAlbumData("token")
	if err == nil {
		t.Fatal("GetAlbumData() error = nil, want request failure")
	}
}

func TestRipTrackEmitsFailureWhenLiteServerMissing(t *testing.T) {
	var output bytes.Buffer
	r := NewRunner(config.ConfigSet{})
	r.Events = events.New(&output)
	track := &model.Track{Type: "songs", ID: "42", SaveDir: t.TempDir()}
	r.ripTrack(track, "token", "token")
	r.Events.Close()

	var seenStarted, seenFailed bool
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		var event events.Event
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatalf("invalid event: %v", err)
		}
		if event.Event == "item_started" {
			seenStarted = true
		}
		if event.Event == "item_failed" {
			seenFailed = true
		}
	}
	if !seenStarted || !seenFailed || r.State.Counter.Error != 1 {
		t.Fatalf("missing failure contract: started=%v failed=%v errors=%d", seenStarted, seenFailed, r.State.Counter.Error)
	}
}

func TestRunv5ExtMvDataMissingFile(t *testing.T) {
	err := runv5.ExtMvData("key-and-urls", filepath.Join(t.TempDir(), "missing.mp4"))
	if err == nil {
		t.Fatal("ExtMvData() error = nil, want missing file failure")
	}
}

func TestRipTrackCountsAACLCMissingLiteServer(t *testing.T) {
	r := NewRunner(config.ConfigSet{})
	r.Config.LiteServer = ""

	track := &model.Track{Type: "songs", ID: "1", SaveDir: t.TempDir()}
	r.ripTrack(track, "token", "token")
	if r.State.Counter.Error != 1 {
		t.Fatalf("r.State.Counter.Error = %d, want 1", r.State.Counter.Error)
	}
}

func TestRipTrackFailsMVWithoutLiteServer(t *testing.T) {
	r := NewRunner(config.ConfigSet{})
	r.Config.LiteServer = ""

	track := &model.Track{Type: "music-videos", ID: "1", SaveDir: t.TempDir()}
	r.ripTrack(track, "token", "token")
	if r.State.Counter.Error != 1 {
		t.Fatalf("r.State.Counter.Error = %d, want 1", r.State.Counter.Error)
	}
}
