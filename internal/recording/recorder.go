// Package recording writes terminal sessions to asciinema v2 (.cast) files.
// Format: https://docs.asciinema.org/manual/asciicast/v2/
package recording

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	flushInterval = 2 * time.Second
	bufSize       = 64 * 1024
)

// header is the asciinema v2 file header (line 1).
type header struct {
	Version   int    `json:"version"`
	Width     uint32 `json:"width"`
	Height    uint32 `json:"height"`
	Timestamp int64  `json:"timestamp"`
	Title     string `json:"title,omitempty"`
}

// Recorder writes SSH stdout to an asciinema v2 .cast file.
// Safe for concurrent use: WriteOutput and WriteResize may be called from
// separate goroutines.
type Recorder struct {
	mu        sync.Mutex
	w         *bufio.Writer
	f         *os.File
	startTime time.Time
	flushStop chan struct{}
	closeOnce sync.Once
}

// New creates (or truncates) the file at path and writes the asciinema v2 header.
// Initial terminal size is cols×rows; the first resize event updates it in the stream.
func New(path string, cols, rows uint32, title string) (*Recorder, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("recording: create file %q: %w", path, err)
	}

	w := bufio.NewWriterSize(f, bufSize)
	now := time.Now()

	h := header{
		Version:   2,
		Width:     cols,
		Height:    rows,
		Timestamp: now.Unix(),
		Title:     title,
	}
	hb, _ := json.Marshal(h)
	if _, err := fmt.Fprintf(w, "%s\n", hb); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("recording: write header: %w", err)
	}

	r := &Recorder{
		w:         w,
		f:         f,
		startTime: now,
		flushStop: make(chan struct{}),
	}

	// Background flush so data reaches disk even if Close is delayed.
	go r.flushLoop()

	return r, nil
}

// WriteOutput records a terminal output event (type "o").
// Safe to call from multiple goroutines.
func (r *Recorder) WriteOutput(data []byte) {
	if len(data) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writeEvent("o", string(data))
}

// WriteResize records a terminal resize event (type "r").
// Safe to call from multiple goroutines.
func (r *Recorder) WriteResize(cols, rows uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writeEvent("r", fmt.Sprintf("%dx%d", cols, rows))
}

// Close flushes buffered data and closes the underlying file.
// Subsequent calls are no-ops.
func (r *Recorder) Close() error {
	var closeErr error
	r.closeOnce.Do(func() {
		close(r.flushStop)
		r.mu.Lock()
		defer r.mu.Unlock()
		if err := r.w.Flush(); err != nil {
			closeErr = fmt.Errorf("recording: flush: %w", err)
		}
		if err := r.f.Close(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("recording: close file: %w", err)
		}
	})
	return closeErr
}

// writeEvent appends one JSON event line. Must be called with r.mu held.
func (r *Recorder) writeEvent(eventType, data string) {
	elapsed := time.Since(r.startTime).Seconds()
	// asciinema v2 event: [elapsed_seconds, event_type, data]
	line, _ := json.Marshal([]any{elapsed, eventType, data})
	_, _ = fmt.Fprintf(r.w, "%s\n", line)
}

func (r *Recorder) flushLoop() {
	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.flushStop:
			return
		case <-ticker.C:
			r.mu.Lock()
			_ = r.w.Flush()
			r.mu.Unlock()
		}
	}
}
