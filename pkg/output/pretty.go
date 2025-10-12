package output

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"hydr0g3n/pkg/engine"
)

const (
	urlColumnWidth        = 60
	statusColumnWidth     = 8
	sizeColumnWidth       = 10
	latencyColumnWidth    = 12
	similarityColumnWidth = 12
)

// PrettyWriter renders engine results in a simple aligned table.
type PrettyWriter struct {
	mu             sync.Mutex
	w              io.Writer
	headerPrinted  bool
	flusher        func() error
	showSimilarity bool
	tableFormat    string
}

// NewPrettyWriter returns a PrettyWriter that writes to w.
func NewPrettyWriter(w io.Writer, showSimilarity bool) *PrettyWriter {
	pw := &PrettyWriter{
		w:              w,
		showSimilarity: showSimilarity,
		tableFormat:    buildTableFormat(showSimilarity),
	}

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

	values := []interface{}{
		truncate(res.URL, urlColumnWidth),
		status,
		size,
		latency,
	}
	if p.showSimilarity {
		values = append(values, formatSimilarity(res))
	}

	if _, err := fmt.Fprintf(p.w, p.tableFormat, values...); err != nil {
		return err
	}

	if p.flusher != nil {
		return p.flusher()
	}

	return nil
}

func (p *PrettyWriter) printHeader() error {
	headers := []string{"URL", "STATUS", "SIZE", "LATENCY"}
	if p.showSimilarity {
		headers = append(headers, "SIMILARITY")
	}

	if _, err := fmt.Fprintf(p.w, p.tableFormat, stringInterfaces(headers)...); err != nil {
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

func formatSimilarity(res engine.Result) string {
	if !res.HasSimilarity {
		return "-"
	}
	return fmt.Sprintf("%.3f", res.Similarity)
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

func buildTableFormat(showSimilarity bool) string {
	widths := []int{urlColumnWidth, statusColumnWidth, sizeColumnWidth, latencyColumnWidth}
	if showSimilarity {
		widths = append(widths, similarityColumnWidth)
	}

	var builder strings.Builder
	for i, width := range widths {
		fmt.Fprintf(&builder, "%%-%ds", width)
		if i < len(widths)-1 {
			builder.WriteString("  ")
		} else {
			builder.WriteByte('\n')
		}
	}
	return builder.String()
}

func stringInterfaces(values []string) []interface{} {
	out := make([]interface{}, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
