package output

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"hydr0g3n/pkg/engine"
)

type BurpWriter struct {
	mu      sync.Mutex
	enc     *xml.Encoder
	flush   func() error
	closer  io.Closer
	writer  io.Writer
	started bool
	closed  bool
	method  string
}

type burpHost struct {
	Name string `xml:",chardata"`
}

type burpMessage struct {
	Base64 string `xml:"base64,attr,omitempty"`
	Value  string `xml:",chardata"`
}

type burpItem struct {
	XMLName        xml.Name    `xml:"item"`
	Time           string      `xml:"time"`
	URL            string      `xml:"url"`
	Host           burpHost    `xml:"host"`
	Port           int         `xml:"port"`
	Protocol       string      `xml:"protocol"`
	Method         string      `xml:"method"`
	Path           string      `xml:"path"`
	Extension      string      `xml:"extension,omitempty"`
	Request        burpMessage `xml:"request"`
	Response       burpMessage `xml:"response,omitempty"`
	Status         int         `xml:"status"`
	ResponseLength int         `xml:"responseLength"`
	Comment        string      `xml:"comment,omitempty"`
}

func NewBurpWriter(w io.Writer, method string) *BurpWriter {
	bw := bufio.NewWriter(w)
	enc := xml.NewEncoder(bw)
	enc.Indent("", "  ")

	return &BurpWriter{
		enc:    enc,
		flush:  bw.Flush,
		writer: bw,
		method: strings.ToUpper(strings.TrimSpace(method)),
	}
}

func NewBurpFile(path, method string) (*BurpWriter, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create burp export: %w", err)
	}

	writer := NewBurpWriter(file, method)
	writer.closer = file

	return writer, nil
}

func (b *BurpWriter) Write(res engine.Result) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("burp writer already closed")
	}

	if err := b.ensureHeader(); err != nil {
		return err
	}

	item, err := buildBurpItem(res, b.method)
	if err != nil {
		return err
	}

	if err := b.enc.Encode(item); err != nil {
		return err
	}

	if err := b.enc.Flush(); err != nil {
		return err
	}

	if b.flush != nil {
		if err := b.flush(); err != nil {
			return err
		}
	}

	return nil
}

func (b *BurpWriter) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	if err := b.ensureHeader(); err != nil {
		return err
	}

	if err := b.enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: "items"}}); err != nil {
		return err
	}

	if err := b.enc.Flush(); err != nil {
		return err
	}

	if b.flush != nil {
		if err := b.flush(); err != nil {
			return err
		}
	}

	b.closed = true

	if b.closer != nil {
		return b.closer.Close()
	}

	return nil
}

func (b *BurpWriter) ensureHeader() error {
	if b.started {
		return nil
	}

	if _, err := io.WriteString(b.writer, xml.Header); err != nil {
		return err
	}

	if err := b.enc.EncodeToken(xml.StartElement{Name: xml.Name{Local: "items"}}); err != nil {
		return err
	}

	if err := b.enc.Flush(); err != nil {
		return err
	}

	b.started = true
	return nil
}

func buildBurpItem(res engine.Result, defaultMethod string) (burpItem, error) {
	parsed, err := url.Parse(res.URL)
	if err != nil {
		return burpItem{}, fmt.Errorf("parse url: %w", err)
	}

	host := parsed.Hostname()
	port := parsed.Port()
	protocol := strings.ToLower(parsed.Scheme)

	if protocol == "" {
		protocol = "http"
	}

	portNumber := 0
	if port == "" {
		switch protocol {
		case "https":
			portNumber = 443
		default:
			portNumber = 80
		}
	} else {
		n, convErr := strconv.Atoi(port)
		if convErr != nil {
			return burpItem{}, fmt.Errorf("parse port: %w", convErr)
		}
		portNumber = n
	}

	method := strings.ToUpper(strings.TrimSpace(res.RequestMethod))
	if method == "" {
		method = strings.ToUpper(strings.TrimSpace(defaultMethod))
	}
	if method == "" {
		method = http.MethodHead
	}

	requestProto := strings.TrimSpace(res.RequestProto)
	if requestProto == "" {
		requestProto = "HTTP/1.1"
	}

	requestURI := parsed.RequestURI()
	if requestURI == "" {
		requestURI = "/"
	}
	if res.RequestURL != "" {
		if reqParsed, parseErr := url.Parse(res.RequestURL); parseErr == nil {
			if uri := reqParsed.RequestURI(); uri != "" {
				requestURI = uri
			}
		}
	}

	reqHeaders := copyHeader(res.RequestHeader)
	if reqHeaders == nil {
		reqHeaders = make(http.Header)
	}

	hostHeader := strings.TrimSpace(res.RequestHost)
	if hostHeader == "" {
		hostHeader = parsed.Host
		if hostHeader == "" && host != "" {
			hostHeader = host
		}
	}

	if hostHeader != "" && reqHeaders.Get("Host") == "" {
		reqHeaders.Set("Host", hostHeader)
	}

	reqBuilder := &strings.Builder{}
	fmt.Fprintf(reqBuilder, "%s %s %s\r\n", method, requestURI, requestProto)
	writeHeaders(reqBuilder, reqHeaders)
	reqBuilder.WriteString("\r\n")

	requestPayload := base64.StdEncoding.EncodeToString([]byte(reqBuilder.String()))

	status := res.StatusCode
	statusLine := strings.TrimSpace(res.ResponseStatus)
	if statusLine == "" {
		statusText := http.StatusText(status)
		if statusText != "" {
			statusLine = fmt.Sprintf("%d %s", status, statusText)
		} else if status != 0 {
			statusLine = strconv.Itoa(status)
		} else {
			statusLine = "0"
		}
	}

	responseProto := strings.TrimSpace(res.ResponseProto)
	if responseProto == "" {
		responseProto = "HTTP/1.1"
	}

	responseBody := res.Body
	responseHeaders := copyHeader(res.ResponseHeader)
	if responseHeaders == nil {
		responseHeaders = make(http.Header)
	}
	if responseHeaders.Get("Content-Length") == "" {
		responseHeaders.Set("Content-Length", strconv.Itoa(len(responseBody)))
	}

	respBuilder := &strings.Builder{}
	fmt.Fprintf(respBuilder, "%s %s\r\n", responseProto, statusLine)
	writeHeaders(respBuilder, responseHeaders)
	respBuilder.WriteString("\r\n")
	if len(responseBody) > 0 {
		respBuilder.Write(responseBody)
	}

	responsePayload := base64.StdEncoding.EncodeToString([]byte(respBuilder.String()))

	responseLength := int(res.ContentLength)
	if responseLength < 0 || (responseLength == 0 && len(responseBody) > 0) {
		responseLength = len(responseBody)
	}

	item := burpItem{
		Time:           time.Now().Format(time.RFC3339),
		URL:            res.URL,
		Host:           burpHost{Name: host},
		Port:           portNumber,
		Protocol:       protocol,
		Method:         method,
		Path:           requestURI,
		Request:        burpMessage{Base64: "true", Value: requestPayload},
		Status:         status,
		ResponseLength: responseLength,
	}

	if len(responseBody) > 0 || status != 0 || len(responseHeaders) > 0 {
		item.Response = burpMessage{Base64: "true", Value: responsePayload}
	}

	return item, nil
}

type burpFindingMessage struct {
	Base64 bool   `json:"base64"`
	Value  string `json:"value"`
}

type burpFinding struct {
	Time           string              `json:"time"`
	URL            string              `json:"url"`
	Host           string              `json:"host"`
	Port           int                 `json:"port"`
	Protocol       string              `json:"protocol"`
	Method         string              `json:"method"`
	Path           string              `json:"path"`
	Extension      string              `json:"extension,omitempty"`
	Request        burpFindingMessage  `json:"request"`
	Response       *burpFindingMessage `json:"response,omitempty"`
	Status         int                 `json:"status"`
	ResponseLength int                 `json:"response_length"`
	Comment        string              `json:"comment,omitempty"`
}

func newBurpFinding(item burpItem) burpFinding {
	finding := burpFinding{
		Time:           item.Time,
		URL:            item.URL,
		Host:           item.Host.Name,
		Port:           item.Port,
		Protocol:       item.Protocol,
		Method:         item.Method,
		Path:           item.Path,
		Extension:      item.Extension,
		Request:        burpFindingMessage{Base64: strings.EqualFold(item.Request.Base64, "true"), Value: item.Request.Value},
		Status:         item.Status,
		ResponseLength: item.ResponseLength,
		Comment:        item.Comment,
	}

	if item.Response.Value != "" || item.Response.Base64 != "" {
		finding.Response = &burpFindingMessage{
			Base64: strings.EqualFold(item.Response.Base64, "true"),
			Value:  item.Response.Value,
		}
	}

	return finding
}

type BurpPoster struct {
	endpoint string
	method   string
	client   *http.Client
}

func NewBurpPoster(host, method string) (*BurpPoster, error) {
	endpoint, err := normalizeBurpEndpoint(host)
	if err != nil {
		return nil, err
	}
	if endpoint == "" {
		return nil, nil
	}

	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod == "" {
		normalizedMethod = http.MethodHead
	}

	return &BurpPoster{
		endpoint: endpoint,
		method:   normalizedMethod,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}, nil
}

func (b *BurpPoster) Write(res engine.Result) error {
	if b == nil {
		return nil
	}

	item, err := buildBurpItem(res, b.method)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(newBurpFinding(item))
	if err != nil {
		return fmt.Errorf("marshal burp finding: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, b.endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create burp request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("send burp finding: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		snippet := strings.TrimSpace(string(body))
		if snippet != "" {
			return fmt.Errorf("burp host %s responded with %s: %s", b.endpoint, resp.Status, snippet)
		}
		return fmt.Errorf("burp host %s responded with %s", b.endpoint, resp.Status)
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func normalizeBurpEndpoint(host string) (string, error) {
	trimmed := strings.TrimSpace(host)
	if trimmed == "" {
		return "", nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		parsed, err = url.Parse("http://" + trimmed)
		if err != nil {
			return "", fmt.Errorf("parse burp host: %w", err)
		}
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("burp host %q is missing a hostname", host)
	}

	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}

	if parsed.Path == "" {
		parsed.Path = "/"
	}

	return parsed.String(), nil
}

func copyHeader(h http.Header) http.Header {
	if len(h) == 0 {
		return nil
	}

	dup := make(http.Header, len(h))
	for key, values := range h {
		if len(values) == 0 {
			dup[key] = nil
			continue
		}

		copied := make([]string, len(values))
		copy(copied, values)
		dup[key] = copied
	}

	return dup
}

func writeHeaders(builder *strings.Builder, headers http.Header) {
	if len(headers) == 0 {
		return
	}

	keys := make([]string, 0, len(headers))
	for key := range headers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		values := headers[key]
		if len(values) == 0 {
			fmt.Fprintf(builder, "%s:\r\n", key)
			continue
		}
		for _, value := range values {
			fmt.Fprintf(builder, "%s: %s\r\n", key, value)
		}
	}
}
