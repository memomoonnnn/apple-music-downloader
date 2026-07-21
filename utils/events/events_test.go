package events

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestEncodeEventWritesOneJSONLRecord(t *testing.T) {
	var output bytes.Buffer
	err := encodeEvent(&output, event{
		SchemaVersion: 1,
		Event:         "progress",
		Sequence:      3,
		RunID:         "run-1",
		Data:          progressData(progress{Phase: "downloading", Completed: 5, Total: 20}, true),
	})
	if err != nil {
		t.Fatalf("encodeEvent returned error: %v", err)
	}
	if output.Bytes()[len(output.Bytes())-1] != '\n' {
		t.Fatal("JSONL record must end with a newline")
	}
	var decoded event
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("event is not valid JSON: %v", err)
	}
	if decoded.Event != "progress" || decoded.Sequence != 3 || decoded.RunID != "run-1" {
		t.Fatalf("unexpected event: %+v", decoded)
	}
}

func TestProgressDataReportsUnknownTotalsWithoutFraction(t *testing.T) {
	data := progressData(progress{Phase: "decrypting", Completed: 7, Total: -1}, false)
	if data.Phase != "decrypting" || data.CompletedBytes != 7 || data.TotalBytes != nil || data.MadeProgress {
		t.Fatalf("unexpected progress data: %+v", data)
	}
	if data.Fraction != nil {
		t.Fatalf("unknown total must not produce a fraction: %v", data.Fraction)
	}
}

func TestBeginProgressWaitsForHeartbeat(t *testing.T) {
	var output bytes.Buffer
	state.Lock()
	state.enabled = true
	state.output = &output
	state.runID = "run-1"
	state.sequence = 0
	state.current = progress{}
	state.Unlock()
	defer func() {
		state.Lock()
		state.enabled = false
		state.output = nil
		state.current = progress{}
		state.Unlock()
	}()

	BeginProgress("downloading", 100)
	if output.Len() != 0 {
		t.Fatalf("progress must wait for the heartbeat, got %s", output.String())
	}
}

func TestContentIncludesPlaylistTitleOnlyWhenSet(t *testing.T) {
	encoded, err := json.Marshal(Content{ID: "song-1", PlaylistTitle: "My Playlist"})
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}
	if !bytes.Contains(encoded, []byte(`"playlist_title":"My Playlist"`)) {
		t.Fatalf("playlist title missing from JSON: %s", encoded)
	}
	encoded, err = json.Marshal(Content{ID: "song-1"})
	if err != nil {
		t.Fatalf("marshal content without playlist: %v", err)
	}
	if bytes.Contains(encoded, []byte("playlist_title")) {
		t.Fatalf("non-playlist content must omit playlist title: %s", encoded)
	}
}
