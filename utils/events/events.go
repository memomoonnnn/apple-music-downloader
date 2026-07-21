package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"

	"main/utils/contract"
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

type event struct {
	SchemaVersion int         `json:"schema_version"`
	Event         string      `json:"event"`
	Sequence      uint64      `json:"sequence"`
	Timestamp     time.Time   `json:"timestamp"`
	RunID         string      `json:"run_id"`
	ItemID        string      `json:"item_id,omitempty"`
	Data          interface{} `json:"data,omitempty"`
}

type progress struct {
	Phase     string
	Completed int64
	Total     int64
	LastSent  int64
}

var state = struct {
	sync.Mutex
	enabled  bool
	output   io.Writer
	runID    string
	sequence uint64
	current  progress
	stop     chan struct{}
	done     chan struct{}
}{}

func EnableJSONL() {
	state.Lock()
	defer state.Unlock()
	if state.enabled {
		return
	}
	state.enabled = true
	state.output = os.Stdout
	state.runID = newRunID()
	state.stop = make(chan struct{})
	state.done = make(chan struct{})
	reader, writer, err := os.Pipe()
	if err != nil {
		state.enabled = false
		close(state.done)
		return
	}
	original := os.Stdout
	os.Stdout = writer
	go discardLegacyOutput(reader, writer, original, state.stop, state.done)
	go sendHeartbeats(state.stop)
}

func Close() {
	state.Lock()
	if !state.enabled {
		state.Unlock()
		return
	}
	close(state.stop)
	state.enabled = false
	state.current = progress{}
	state.Unlock()
	<-state.done
}

func Enabled() bool {
	state.Lock()
	defer state.Unlock()
	return state.enabled
}

func RunStarted(urls []string, format string) {
	emit("run_started", "", struct {
		URLs   []string `json:"urls"`
		Format string   `json:"format"`
	}{urls, format})
}

func RunCompleted(status string, completed, warnings, failures int) {
	EndProgress()
	emit("run_completed", "", struct {
		Status    string `json:"status"`
		Completed int    `json:"completed"`
		Warnings  int    `json:"warnings"`
		Failures  int    `json:"failures"`
	}{status, completed, warnings, failures})
}

func ItemStarted(content Content) {
	EndProgress()
	emit("item_started", content.ID, struct {
		Content Content `json:"content"`
	}{content})
}

func ItemCompleted(itemID, outputPath string) {
	EndProgress()
	emit("item_completed", itemID, struct {
		OutputPath string `json:"output_path"`
	}{outputPath})
}

func ItemFailed(itemID, code string, err error) {
	EndProgress()
	message := ""
	if err != nil {
		message = contract.RedactSensitiveText(err.Error())
	}
	emit("item_failed", itemID, struct {
		Code    string `json:"code"`
		Message string `json:"message,omitempty"`
	}{code, message})
}

func Diagnostic(level, code, message string) {
	emit("diagnostic", "", struct {
		Level   string `json:"level"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}{level, code, contract.RedactSensitiveText(message)})
}

func BeginProgress(phase string, total int64) {
	state.Lock()
	if !state.enabled {
		state.Unlock()
		return
	}
	state.current = progress{Phase: phase, Total: total}
	state.Unlock()
}

func UpdateProgress(completed int64) {
	state.Lock()
	if !state.enabled || state.current.Phase == "" {
		state.Unlock()
		return
	}
	state.current.Completed = completed
	state.Unlock()
}

func EndProgress() {
	state.Lock()
	state.current = progress{}
	state.Unlock()
}

type progressWriter struct {
	total     int64
	completed int64
}

func NewProgressWriter(phase string, total int64) io.Writer {
	BeginProgress(phase, total)
	return &progressWriter{total: total}
}

func (w *progressWriter) Write(p []byte) (int, error) {
	w.completed += int64(len(p))
	UpdateProgress(w.completed)
	return len(p), nil
}

func sendHeartbeats(stop <-chan struct{}) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			state.Lock()
			if state.enabled && state.current.Phase != "" {
				emitLocked("progress", "", progressData(state.current, state.current.Completed != state.current.LastSent))
				state.current.LastSent = state.current.Completed
			}
			state.Unlock()
		case <-stop:
			return
		}
	}
}

type progressDataPayload struct {
	Phase          string   `json:"phase"`
	CompletedBytes int64    `json:"completed_bytes"`
	TotalBytes     *int64   `json:"total_bytes,omitempty"`
	Fraction       *float64 `json:"fraction,omitempty"`
	MadeProgress   bool     `json:"made_progress"`
}

func progressData(value progress, madeProgress bool) progressDataPayload {
	data := progressDataPayload{Phase: value.Phase, CompletedBytes: value.Completed, MadeProgress: madeProgress}
	if value.Total > 0 {
		total := value.Total
		fraction := float64(value.Completed) / float64(value.Total)
		data.TotalBytes = &total
		data.Fraction = &fraction
	}
	return data
}

func emit(name, itemID string, data interface{}) {
	state.Lock()
	defer state.Unlock()
	emitLocked(name, itemID, data)
}

func emitLocked(name, itemID string, data interface{}) {
	if !state.enabled {
		return
	}
	state.sequence++
	_ = encodeEvent(state.output, event{
		SchemaVersion: 1,
		Event:         name,
		Sequence:      state.sequence,
		Timestamp:     time.Now().UTC(),
		RunID:         state.runID,
		ItemID:        itemID,
		Data:          data,
	})
}

func encodeEvent(output io.Writer, value event) error {
	return json.NewEncoder(output).Encode(value)
}

func discardLegacyOutput(reader *os.File, writer *os.File, original *os.File, stop <-chan struct{}, done chan<- struct{}) {
	go func() {
		<-stop
		_ = writer.Close()
	}()
	_, _ = io.Copy(io.Discard, reader)
	os.Stdout = original
	_ = reader.Close()
	close(done)
}

func newRunID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return time.Now().UTC().Format("20060102T150405.000000000")
}
