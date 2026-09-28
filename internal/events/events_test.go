package events

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestEmitterWritesOrderedJSONL(t *testing.T) {
	var output bytes.Buffer
	emitter := New(&output)
	emitter.Emit("run_started", "", map[string]any{"format": "alac"})
	emitter.Emit("item_started", "42", map[string]any{"content": Content{ID: "42", PlaylistTitle: "List"}})
	emitter.Close()

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %q", len(lines), output.String())
	}
	for index, line := range lines {
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("line %d is not JSON: %v", index, err)
		}
		if event.SchemaVersion != 1 || event.Sequence != uint64(index+1) || event.RunID == "" {
			t.Fatalf("invalid envelope: %+v", event)
		}
	}
}

func TestRedactAuthenticationValues(t *testing.T) {
	message := `authorization-token: Bearer abc.def media-user-token=secret`
	redacted := Redact(message)
	if strings.Contains(redacted, "abc.def") || strings.Contains(redacted, "secret") {
		t.Fatalf("authentication value leaked: %q", redacted)
	}
}
