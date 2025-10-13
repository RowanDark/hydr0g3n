package output

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-isatty"

	"hydr0g3n/pkg/engine"
)

const (
	urlColumnWidth        = 60
	statusColumnWidth     = 8
	sizeColumnWidth       = 10
	latencyColumnWidth    = 12
	similarityColumnWidth = 12
)

// ViewMode controls how pretty output is rendered.
type ViewMode int

const (
	// ViewModeTable renders results in a padded table.
	ViewModeTable ViewMode = iota
	// ViewModeTree renders results in a hierarchical tree.
	ViewModeTree
)

// ParseViewMode validates and returns a ViewMode.
func ParseViewMode(v string) (ViewMode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "table":
		return ViewModeTable, nil
	case "tree":
		return ViewModeTree, nil
	default:
		return ViewModeTable, fmt.Errorf("unknown view mode %q", v)
	}
}

// ColorMode controls whether ANSI color is applied to pretty output.
type ColorMode int

const (
	// ColorModeAuto only uses color when writing to an interactive terminal.
	ColorModeAuto ColorMode = iota
	// ColorModeAlways always emits color escape sequences.
	ColorModeAlways
	// ColorModeNever never emits color escape sequences.
	ColorModeNever
)

// ParseColorMode validates and returns a ColorMode.
func ParseColorMode(v string) (ColorMode, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "auto":
		return ColorModeAuto, nil
	case "always":
		return ColorModeAlways, nil
	case "never":
		return ColorModeNever, nil
	default:
		return ColorModeAuto, fmt.Errorf("unknown color mode %q", v)
	}
}

// ColorPreset identifies a named color palette for pretty output.
type ColorPreset string

const (
	// ColorPresetDefault is the default palette optimized for balanced contrast.
	ColorPresetDefault ColorPreset = "default"
	// ColorPresetProtanopia emphasizes cyan and amber contrasts.
	ColorPresetProtanopia ColorPreset = "protanopia"
	// ColorPresetTritanopia emphasizes warm vs cool contrasts.
	ColorPresetTritanopia ColorPreset = "tritanopia"
	// ColorPresetBlueLight uses warmer tones for late-night sessions.
	ColorPresetBlueLight ColorPreset = "blue-light"
)

// ParseColorPreset validates and returns a ColorPreset, defaulting to ColorPresetDefault.
func ParseColorPreset(v string) (ColorPreset, error) {
	value := strings.ToLower(strings.TrimSpace(v))
	if value == "" {
		return ColorPresetDefault, nil
	}
	switch ColorPreset(value) {
	case ColorPresetDefault, ColorPresetProtanopia, ColorPresetTritanopia, ColorPresetBlueLight:
		return ColorPreset(value), nil
	default:
		return ColorPresetDefault, fmt.Errorf("unknown color preset %q", v)
	}
}

type colorPalette struct {
	Reset           string
	Header          string
	Path            string
	StatusOK        string
	StatusRedirect  string
	StatusClientErr string
	StatusServerErr string
	StatusOther     string
	StatusError     string
	Size            string
	Latency         string
	Similarity      string
	TreeLine        string
}

var paletteCatalog = map[ColorPreset]colorPalette{
	ColorPresetDefault: {
		Reset:           "\u001b[0m",
		Header:          "\u001b[38;5;252m",
		Path:            "\u001b[38;5;45m",
		StatusOK:        "\u001b[38;5;113m",
		StatusRedirect:  "\u001b[38;5;178m",
		StatusClientErr: "\u001b[38;5;208m",
		StatusServerErr: "\u001b[38;5;204m",
		StatusOther:     "\u001b[38;5;111m",
		StatusError:     "\u001b[38;5;203m",
		Size:            "\u001b[38;5;250m",
		Latency:         "\u001b[38;5;246m",
		Similarity:      "\u001b[38;5;147m",
		TreeLine:        "\u001b[38;5;240m",
	},
	ColorPresetProtanopia: {
		Reset:           "\u001b[0m",
		Header:          "\u001b[38;5;252m",
		Path:            "\u001b[38;5;33m",
		StatusOK:        "\u001b[38;5;80m",
		StatusRedirect:  "\u001b[38;5;220m",
		StatusClientErr: "\u001b[38;5;172m",
		StatusServerErr: "\u001b[38;5;125m",
		StatusOther:     "\u001b[38;5;110m",
		StatusError:     "\u001b[38;5;160m",
		Size:            "\u001b[38;5;252m",
		Latency:         "\u001b[38;5;247m",
		Similarity:      "\u001b[38;5;153m",
		TreeLine:        "\u001b[38;5;244m",
	},
	ColorPresetTritanopia: {
		Reset:           "\u001b[0m",
		Header:          "\u001b[38;5;253m",
		Path:            "\u001b[38;5;75m",
		StatusOK:        "\u001b[38;5;34m",
		StatusRedirect:  "\u001b[38;5;179m",
		StatusClientErr: "\u001b[38;5;202m",
		StatusServerErr: "\u001b[38;5;132m",
		StatusOther:     "\u001b[38;5;111m",
		StatusError:     "\u001b[38;5;160m",
		Size:            "\u001b[38;5;250m",
		Latency:         "\u001b[38;5;248m",
		Similarity:      "\u001b[38;5;152m",
		TreeLine:        "\u001b[38;5;242m",
	},
	ColorPresetBlueLight: {
		Reset:           "\u001b[0m",
		Header:          "\u001b[38;5;251m",
		Path:            "\u001b[38;5;67m",
		StatusOK:        "\u001b[38;5;71m",
		StatusRedirect:  "\u001b[38;5;172m",
		StatusClientErr: "\u001b[38;5;208m",
		StatusServerErr: "\u001b[38;5;139m",
		StatusOther:     "\u001b[38;5;111m",
		StatusError:     "\u001b[38;5;131m",
		Size:            "\u001b[38;5;250m",
		Latency:         "\u001b[38;5;247m",
		Similarity:      "\u001b[38;5;151m",
		TreeLine:        "\u001b[38;5;242m",
	},
}

// PrettyOptions describes how pretty output should be rendered.
type PrettyOptions struct {
	ShowSimilarity bool
	ViewMode       ViewMode
	ColorMode      ColorMode
	ColorPreset    ColorPreset
	TargetURL      string
}

// PrettyWriter renders engine results using the configured view mode.
type PrettyWriter struct {
	mu            sync.Mutex
	w             io.Writer
	flusher       func() error
	headerPrinted bool

	opts         PrettyOptions
	palette      colorPalette
	colorEnabled bool
	tableWidths  []int
	tree         *treePrinter
}

// NewPrettyWriter returns a PrettyWriter that writes to w.
func NewPrettyWriter(w io.Writer, opts PrettyOptions) *PrettyWriter {
	writer := &PrettyWriter{
		w:           w,
		opts:        opts,
		palette:     paletteCatalog[ColorPresetDefault],
		tableWidths: []int{urlColumnWidth, statusColumnWidth, sizeColumnWidth, latencyColumnWidth},
	}

	if palette, ok := paletteCatalog[opts.ColorPreset]; ok {
		writer.palette = palette
	}

	if opts.ShowSimilarity {
		writer.tableWidths = append(writer.tableWidths, similarityColumnWidth)
	}

	if f, ok := w.(interface{ Flush() error }); ok {
		writer.flusher = f.Flush
	}

	writer.colorEnabled = shouldEnableColor(opts.ColorMode, w)

	if opts.ViewMode == ViewModeTree {
		writer.tree = newTreePrinter(opts.TargetURL)
	}

	return writer
}

// Write registers a single result. In table mode, rows are emitted immediately. In tree mode, results are stored until Flush.
func (p *PrettyWriter) Write(res engine.Result) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.opts.ViewMode == ViewModeTree {
		if p.tree != nil {
			p.tree.add(res)
		}
		return nil
	}

	return p.writeTableRow(res)
}

// Flush finalizes the view and writes buffered content, if any.
func (p *PrettyWriter) Flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.opts.ViewMode == ViewModeTree {
		if p.tree != nil {
			if err := p.printTree(); err != nil {
				return err
			}
		}
	}

	if p.flusher != nil {
		return p.flusher()
	}

	return nil
}

func (p *PrettyWriter) writeTableRow(res engine.Result) error {
	if !p.headerPrinted {
		if err := p.printTableHeader(); err != nil {
			return err
		}
		p.headerPrinted = true
	}

	row := p.renderTableRow(res)
	if _, err := io.WriteString(p.w, row); err != nil {
		return err
	}

	if p.flusher != nil {
		return p.flusher()
	}

	return nil
}

func (p *PrettyWriter) printTableHeader() error {
	headers := []string{"URL", "STATUS", "SIZE", "LATENCY"}
	if p.opts.ShowSimilarity {
		headers = append(headers, "SIMILARITY")
	}

	var builder strings.Builder
	for i, header := range headers {
		formatted := fmt.Sprintf("%-*s", p.tableWidths[i], header)
		if p.colorEnabled && p.palette.Header != "" {
			formatted = wrapColor(formatted, p.palette.Header, p.palette.Reset)
		}
		builder.WriteString(formatted)
		if i < len(headers)-1 {
			builder.WriteString("  ")
		} else {
			builder.WriteByte('\n')
		}
	}

	if _, err := p.w.Write([]byte(builder.String())); err != nil {
		return err
	}

	return nil
}

func (p *PrettyWriter) renderTableRow(res engine.Result) string {
	columns := []string{
		truncate(res.URL, urlColumnWidth),
		formatStatus(res),
		formatSize(res),
		formatLatency(res.Duration),
	}
	if p.opts.ShowSimilarity {
		columns = append(columns, formatSimilarity(res))
	}

	var builder strings.Builder
	for i, col := range columns {
		width := p.tableWidths[i]
		formatted := fmt.Sprintf("%-*s", width, col)
		switch i {
		case 0:
			if p.colorEnabled && p.palette.Path != "" {
				formatted = wrapColor(formatted, p.palette.Path, p.palette.Reset)
			}
		case 1:
			if p.colorEnabled {
				formatted = wrapColor(formatted, p.statusColor(res), p.palette.Reset)
			}
		case 2:
			if p.colorEnabled && p.palette.Size != "" {
				formatted = wrapColor(formatted, p.palette.Size, p.palette.Reset)
			}
		case 3:
			if p.colorEnabled && p.palette.Latency != "" {
				formatted = wrapColor(formatted, p.palette.Latency, p.palette.Reset)
			}
		case 4:
			if p.colorEnabled && p.palette.Similarity != "" {
				formatted = wrapColor(formatted, p.palette.Similarity, p.palette.Reset)
			}
		}
		builder.WriteString(formatted)
		if i < len(columns)-1 {
			builder.WriteString("  ")
		} else {
			builder.WriteByte('\n')
		}
	}

	return builder.String()
}

func (p *PrettyWriter) printTree() error {
	if p.tree == nil {
		return nil
	}

	label := p.tree.rootLabel()
	if p.colorEnabled && p.palette.Path != "" {
		label = wrapColor(label, p.palette.Path, p.palette.Reset)
	}

	if _, err := fmt.Fprintln(p.w, label); err != nil {
		return err
	}

	children := p.tree.children()
	for i, node := range children {
		if err := p.printTreeNode(node, "", i == len(children)-1); err != nil {
			return err
		}
	}

	return nil
}

func (p *PrettyWriter) printTreeNode(node *treeNode, prefix string, isLast bool) error {
	connector := "├── "
	childPrefix := prefix + "│   "
	if isLast {
		connector = "└── "
		childPrefix = prefix + "    "
	}

	linePrefix := prefix + connector
	if p.colorEnabled && p.palette.TreeLine != "" {
		linePrefix = wrapColor(linePrefix, p.palette.TreeLine, p.palette.Reset)
	}

	label := node.name
	if node.result == nil && len(node.children) > 0 && !strings.HasSuffix(label, "/") {
		label += "/"
	}
	if p.colorEnabled && p.palette.Path != "" {
		label = wrapColor(label, p.palette.Path, p.palette.Reset)
	}

	line := linePrefix + label
	if node.result != nil {
		line += " " + p.formatTreeMetrics(*node.result)
	}

	if _, err := fmt.Fprintln(p.w, line); err != nil {
		return err
	}

	ordered := node.orderedChildren()
	for i, child := range ordered {
		if err := p.printTreeNode(child, childPrefix, i == len(ordered)-1); err != nil {
			return err
		}
	}

	return nil
}

func (p *PrettyWriter) formatTreeMetrics(res engine.Result) string {
	status := formatStatus(res)
	size := formatSize(res)
	latency := formatLatency(res.Duration)
	similarity := ""
	if p.opts.ShowSimilarity {
		similarity = formatSimilarity(res)
	}

	if p.colorEnabled {
		status = wrapColor(status, p.statusColor(res), p.palette.Reset)
		size = wrapColor(size, p.palette.Size, p.palette.Reset)
		latency = wrapColor(latency, p.palette.Latency, p.palette.Reset)
		if p.opts.ShowSimilarity {
			similarity = wrapColor(similarity, p.palette.Similarity, p.palette.Reset)
		}
	}

	parts := []string{status, size, latency}
	if p.opts.ShowSimilarity {
		parts = append(parts, similarity)
	}

	return "[" + strings.Join(parts, " • ") + "]"
}

func (p *PrettyWriter) statusColor(res engine.Result) string {
	if res.Err != nil {
		return p.palette.StatusError
	}

	code := res.StatusCode
	switch {
	case code >= 200 && code < 300:
		return p.palette.StatusOK
	case code >= 300 && code < 400:
		return p.palette.StatusRedirect
	case code >= 400 && code < 500:
		return p.palette.StatusClientErr
	case code >= 500 && code < 600:
		return p.palette.StatusServerErr
	case code == 0:
		return p.palette.StatusOther
	default:
		return p.palette.StatusOther
	}
}

func shouldEnableColor(mode ColorMode, w io.Writer) bool {
	switch mode {
	case ColorModeAlways:
		return true
	case ColorModeNever:
		return false
	default:
		if f, ok := w.(interface{ Fd() uintptr }); ok {
			fd := f.Fd()
			return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
		}
		return false
	}
}

func wrapColor(text, color, reset string) string {
	if color == "" || reset == "" {
		return text
	}
	return color + text + reset
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

type treeNode struct {
	name     string
	result   *engine.Result
	children map[string]*treeNode
	order    []string
}

func newTreeNode(name string) *treeNode {
	return &treeNode{name: name, children: make(map[string]*treeNode)}
}

func (n *treeNode) ensureChild(name string) *treeNode {
	if child, ok := n.children[name]; ok {
		return child
	}
	child := newTreeNode(name)
	n.children[name] = child
	n.order = append(n.order, name)
	return child
}

func (n *treeNode) orderedChildren() []*treeNode {
	ordered := make([]*treeNode, 0, len(n.order))
	for _, name := range n.order {
		if child, ok := n.children[name]; ok {
			ordered = append(ordered, child)
		}
	}
	return ordered
}

type treePrinter struct {
	root     *treeNode
	rootHost string
}

func newTreePrinter(target string) *treePrinter {
	rootName := strings.TrimSpace(target)
	rootHost := ""
	if parsed, err := url.Parse(target); err == nil && parsed.Host != "" {
		rootHost = parsed.Host
		rootName = parsed.Host
	}
	root := newTreeNode(rootName)
	return &treePrinter{root: root, rootHost: rootHost}
}

func (t *treePrinter) rootLabel() string {
	if t.root == nil || strings.TrimSpace(t.root.name) == "" {
		return "results"
	}
	return t.root.name
}

func (t *treePrinter) add(res engine.Result) {
	if t.root == nil {
		return
	}

	segments := t.pathSegments(res.URL)
	node := t.root
	for _, segment := range segments {
		node = node.ensureChild(segment)
	}
	copy := res
	node.result = &copy
}

func (t *treePrinter) children() []*treeNode {
	if t.root == nil {
		return nil
	}
	return t.root.orderedChildren()
}

func (t *treePrinter) pathSegments(raw string) []string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return []string{strings.TrimSpace(raw)}
	}

	segments := make([]string, 0, 8)
	if parsed.Host != "" && parsed.Host != t.rootHost {
		segments = append(segments, parsed.Host)
	}

	path := strings.Trim(parsed.Path, "/")
	if path != "" {
		segments = append(segments, strings.Split(path, "/")...)
	} else {
		segments = append(segments, "/")
	}

	if parsed.RawQuery != "" && len(segments) > 0 {
		segments[len(segments)-1] = segments[len(segments)-1] + "?" + parsed.RawQuery
	}

	return segments
}
