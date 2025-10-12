package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"hydr0g3n/pkg/engine"
)

// JSONLWriter writes engine results as newline-delimited JSON objects.
type JSONLWriter struct {
	mu     sync.Mutex
	enc    *json.Encoder
	flush  func() error
	closer io.Closer
}

// RunHeader describes metadata emitted as the first JSONL entry for a run.
type RunHeader struct {
	Type      string   `json:"type"`
	RunID     string   `json:"run_id"`
	TargetURL string   `json:"target_url,omitempty"`
	Wordlist  string   `json:"wordlist,omitempty"`
	StartedAt string   `json:"started_at,omitempty"`
	Config    []string `json:"config,omitempty"`
	Payloads  []string `json:"payloads,omitempty"`
}

// NewJSONLWriter returns a JSONLWriter that writes to w.
func NewJSONLWriter(w io.Writer) *JSONLWriter {
	bw := bufio.NewWriter(w)
	return &JSONLWriter{
		enc:   json.NewEncoder(bw),
		flush: bw.Flush,
	}
}

// NewJSONLFile creates a JSONLWriter that manages the lifecycle of the file at path.
func NewJSONLFile(path string) (*JSONLWriter, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create output file: %w", err)
	}

	writer := NewJSONLWriter(file)
	writer.closer = file
	return writer, nil
}

// WriteHeader writes a metadata entry describing the run before any results.
func (j *JSONLWriter) WriteHeader(header RunHeader) error {
	if header.Type == "" {
		header.Type = "run"
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	if err := j.enc.Encode(header); err != nil {
		return err
	}

	if j.flush != nil {
		if err := j.flush(); err != nil {
			return err
		}
	}

	return nil
}

// Write appends a result entry to the stream.
func (j *JSONLWriter) Write(res engine.Result) error {
	entry := struct {
		URL       string  `json:"url"`
		Status    int     `json:"status"`
		Size      int64   `json:"size"`
		LatencyMS float64 `json:"latency_ms"`
		Error     string  `json:"error,omitempty"`
	}{
		URL:    res.URL,
		Status: res.StatusCode,
		Size:   res.ContentLength,
	}

	if res.Duration > 0 {
		entry.LatencyMS = float64(res.Duration) / float64(time.Millisecond)
	}

	if res.Err != nil {
		entry.Error = res.Err.Error()
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	if err := j.enc.Encode(entry); err != nil {
		return err
	}

	if j.flush != nil {
		if err := j.flush(); err != nil {
			return err
		}
	}

	return nil
}

// Close flushes any buffered data and closes the underlying writer when owned.
func (j *JSONLWriter) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.flush != nil {
		if err := j.flush(); err != nil {
			return err
		}
	}

	if j.closer != nil {
		return j.closer.Close()
	}

	return nil
}
