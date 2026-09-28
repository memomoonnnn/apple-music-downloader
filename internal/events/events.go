package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"regexp"
	"sync"
	"time"
)

const heartbeatInterval = 30 * time.Second

type Content struct {
	ID            string `json:"id,omitempty"`
	Kind          string `json:"kind,omitempty"`
	Title         string `json:"title,omitempty"`
	Artist        string `json:"artist,omitempty"`
	Album         string `json:"album,omitempty"`
	PlaylistTitle string `json:"playlist_title,omitempty"`
	Position      int    `json:"position,omitempty"`
	Total         int    `json:"total,omitempty"`
}

type Event struct {
	SchemaVersion int       `json:"schema_version"`
	Event         string    `json:"event"`
	Sequence      uint64    `json:"sequence"`
	Timestamp     time.Time `json:"timestamp"`
	RunID         string    `json:"run_id"`
	ItemID        string    `json:"item_id,omitempty"`
	Data          any       `json:"data,omitempty"`
}

type progress struct {
	phase     string
	completed int64
	total     int64
	lastSent  int64
}

type Emitter struct {
	mu       sync.Mutex
	output   io.Writer
	runID    string
	sequence uint64
	current  progress
	stop     chan struct{}
	done     chan struct{}
}

func New(output io.Writer) *Emitter {
	identifier := make([]byte, 16)
	runID := time.Now().UTC().Format("20060102T150405.000000000")
	if _, err := rand.Read(identifier); err == nil {
		runID = hex.EncodeToString(identifier)
	}
	e := &Emitter{output: output, runID: runID, stop: make(chan struct{}), done: make(chan struct{})}
	go e.heartbeat()
	return e
}

func (e *Emitter) Close() {
	close(e.stop)
	<-e.done
}

func (e *Emitter) Emit(name, itemID string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.emitLocked(name, itemID, data)
}

func (e *Emitter) emitLocked(name, itemID string, data any) {
	e.sequence++
	_ = json.NewEncoder(e.output).Encode(Event{
		SchemaVersion: 1,
		Event:         name,
		Sequence:      e.sequence,
		Timestamp:     time.Now().UTC(),
		RunID:         e.runID,
		ItemID:        itemID,
		Data:          data,
	})
}

func (e *Emitter) BeginProgress(phase string, total int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.current = progress{phase: phase, total: total}
}

func (e *Emitter) UpdateProgress(completed int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.current.phase != "" {
		e.current.completed = completed
	}
}

func (e *Emitter) SetProgress(phase string, completed, total int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.current.phase != phase {
		e.current = progress{phase: phase}
	}
	e.current.completed = completed
	e.current.total = total
}

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization-token\s*[:=]\s*["']?)(?:Bearer\s+)?[^\s"',]+`),
	regexp.MustCompile(`(?i)(media-user-token\s*[:=]\s*["']?)[^\s"',]+`),
	regexp.MustCompile(`(?i)(x-apple-music-user-token\s*[:=]\s*["']?)[^\s"',]+`),
	regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/=-]+`),
}

func Redact(message string) string {
	for _, pattern := range sensitivePatterns {
		message = pattern.ReplaceAllString(message, `${1}<redacted>`)
	}
	return message
}

func (e *Emitter) EndProgress() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.current = progress{}
}

func (e *Emitter) heartbeat() {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	defer close(e.done)
	for {
		select {
		case <-ticker.C:
			e.mu.Lock()
			if e.current.phase != "" {
				current := e.current
				data := map[string]any{
					"phase":           current.phase,
					"completed_bytes": current.completed,
					"made_progress":   current.completed != current.lastSent,
				}
				if current.total > 0 {
					data["total_bytes"] = current.total
					data["fraction"] = float64(current.completed) / float64(current.total)
				}
				e.emitLocked("progress", "", data)
				e.current.lastSent = current.completed
			}
			e.mu.Unlock()
		case <-e.stop:
			return
		}
	}
}
