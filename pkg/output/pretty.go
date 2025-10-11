package output

import (
	"fmt"
	"io"
	"sync"
	"time"

	"hydr0g3n/pkg/engine"
)

const (
	urlColumnWidth     = 60
	statusColumnWidth  = 8
	sizeColumnWidth    = 10
	latencyColumnWidth = 12
)

var tableFormat = fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds\n",
	urlColumnWidth,
	statusColumnWidth,
	sizeColumnWidth,
	latencyColumnWidth,
)

// PrettyWriter renders engine results in a simple aligned table.
type PrettyWriter struct {
	mu            sync.Mutex
	w             io.Writer
	headerPrinted bool
	flusher       func() error
}

// NewPrettyWriter returns a PrettyWriter that writes to w.
func NewPrettyWriter(w io.Writer) *PrettyWriter {
	pw := &PrettyWriter{w: w}

	if f, ok := w.(interface{ Flush() error }); ok {
		pw.flusher = f.Flush
	}

	return pw
}

// Write prints a single result row to the table.
func (p *PrettyWriter) Write(res engine.Result) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.headerPrinted {
		if err := p.printHeader(); err != nil {
			return err
		}
		p.headerPrinted = true
	}

	status := formatStatus(res)
	size := formatSize(res)
	latency := formatLatency(res.Duration)

	if _, err := fmt.Fprintf(p.w, tableFormat,
		truncate(res.URL, urlColumnWidth),
		status,
		size,
		latency,
	); err != nil {
		return err
	}

	if p.flusher != nil {
		return p.flusher()
	}

	return nil
}

func (p *PrettyWriter) printHeader() error {
	if _, err := fmt.Fprintf(p.w, tableFormat, "URL", "STATUS", "SIZE", "LATENCY"); err != nil {
		return err
	}
	if p.flusher != nil {
		return p.flusher()
	}
	return nil
}

func formatStatus(res engine.Result) string {
	if res.Err != nil {
		return "ERR"
	}
	if res.StatusCode == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", res.StatusCode)
}

func formatSize(res engine.Result) string {
	if res.Err != nil {
		return "-"
	}
	if res.ContentLength < 0 {
		return "-"
	}
	return fmt.Sprintf("%d", res.ContentLength)
}

func formatLatency(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	if d < time.Microsecond {
		return d.String()
	}
	return d.Truncate(time.Microsecond).String()
}

func truncate(s string, width int) string {
	if len(s) <= width {
		return s
	}
	if width <= 3 {
		return s[:width]
	}
	return s[:width-3] + "..."
}
