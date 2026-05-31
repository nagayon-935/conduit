package recording

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestRecorder(t *testing.T) (*Recorder, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.cast")
	r, err := New(path, 80, 24, "test")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r, path
}

func readCastLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open cast file: %v", err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

func TestRecorder_Header(t *testing.T) {
	t.Parallel()
	r, path := newTestRecorder(t)
	_ = r.Close()

	lines := readCastLines(t, path)
	if len(lines) < 1 {
		t.Fatal("expected at least 1 line (header)")
	}

	var h header
	if err := json.Unmarshal([]byte(lines[0]), &h); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if h.Version != 2 {
		t.Errorf("version = %d, want 2", h.Version)
	}
	if h.Width != 80 || h.Height != 24 {
		t.Errorf("size = %dx%d, want 80x24", h.Width, h.Height)
	}
	if h.Timestamp == 0 {
		t.Error("timestamp should not be 0")
	}
}

func TestRecorder_WriteOutput(t *testing.T) {
	t.Parallel()
	r, path := newTestRecorder(t)
	r.WriteOutput([]byte("hello world"))
	_ = r.Close()

	lines := readCastLines(t, path)
	if len(lines) < 2 {
		t.Fatalf("expected header + 1 event, got %d lines", len(lines))
	}

	var event []json.RawMessage
	if err := json.Unmarshal([]byte(lines[1]), &event); err != nil {
		t.Fatalf("unmarshal event: %v", err)
	}
	if len(event) != 3 {
		t.Fatalf("event length = %d, want 3", len(event))
	}

	var elapsed float64
	var eType, data string
	_ = json.Unmarshal(event[0], &elapsed)
	_ = json.Unmarshal(event[1], &eType)
	_ = json.Unmarshal(event[2], &data)

	if eType != "o" {
		t.Errorf("event type = %q, want %q", eType, "o")
	}
	if data != "hello world" {
		t.Errorf("data = %q, want %q", data, "hello world")
	}
	if elapsed < 0 {
		t.Errorf("elapsed = %f, want >= 0", elapsed)
	}
}

func TestRecorder_WriteResize(t *testing.T) {
	t.Parallel()
	r, path := newTestRecorder(t)
	r.WriteResize(120, 40)
	_ = r.Close()

	lines := readCastLines(t, path)
	if len(lines) < 2 {
		t.Fatalf("expected header + resize event, got %d lines", len(lines))
	}

	var event []json.RawMessage
	_ = json.Unmarshal([]byte(lines[1]), &event)
	var eType, data string
	_ = json.Unmarshal(event[1], &eType)
	_ = json.Unmarshal(event[2], &data)

	if eType != "r" {
		t.Errorf("event type = %q, want %q", eType, "r")
	}
	if data != "120x40" {
		t.Errorf("data = %q, want %q", data, "120x40")
	}
}

func TestRecorder_EventOrder(t *testing.T) {
	t.Parallel()
	r, path := newTestRecorder(t)
	r.WriteOutput([]byte("first"))
	time.Sleep(5 * time.Millisecond)
	r.WriteOutput([]byte("second"))
	_ = r.Close()

	lines := readCastLines(t, path)
	if len(lines) < 3 {
		t.Fatalf("expected header + 2 events, got %d lines", len(lines))
	}

	parse := func(line string) float64 {
		var ev []json.RawMessage
		_ = json.Unmarshal([]byte(line), &ev)
		var elapsed float64
		_ = json.Unmarshal(ev[0], &elapsed)
		return elapsed
	}

	t1 := parse(lines[1])
	t2 := parse(lines[2])
	if t2 <= t1 {
		t.Errorf("second event elapsed (%f) should be > first (%f)", t2, t1)
	}
}

func TestRecorder_CloseIdempotent(t *testing.T) {
	t.Parallel()
	r, _ := newTestRecorder(t)
	if err := r.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close should be no-op, got: %v", err)
	}
}

func TestRecorder_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	r, path := newTestRecorder(t)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(2)
		go func(n int) {
			defer wg.Done()
			r.WriteOutput([]byte("output"))
			_ = n
		}(i)
		go func(n int) {
			defer wg.Done()
			r.WriteResize(uint32(80+n), 24)
		}(i)
	}
	wg.Wait()
	_ = r.Close()

	lines := readCastLines(t, path)
	// header + 40 output + 20 resize = 61 lines
	if len(lines) < 41 {
		t.Errorf("expected >= 41 lines, got %d", len(lines))
	}
}

func TestRecorder_EmptyWrite(t *testing.T) {
	t.Parallel()
	r, path := newTestRecorder(t)
	r.WriteOutput(nil)
	r.WriteOutput([]byte{})
	_ = r.Close()

	lines := readCastLines(t, path)
	// Only header — empty writes should be no-ops.
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only), got %d", len(lines))
	}
}
