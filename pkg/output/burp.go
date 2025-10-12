package output

import (
	"bufio"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
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

	item, err := b.buildItem(res)
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

func (b *BurpWriter) buildItem(res engine.Result) (burpItem, error) {
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
		if n, convErr := strconv.Atoi(port); convErr == nil {
			portNumber = n
		} else {
			return burpItem{}, fmt.Errorf("parse port: %w", convErr)
		}
	}

	method := b.method
	if method == "" {
		method = http.MethodHead
	}

	requestURI := parsed.RequestURI()
	if requestURI == "" {
		requestURI = "/"
	}

	reqBuilder := &strings.Builder{}
	fmt.Fprintf(reqBuilder, "%s %s HTTP/1.1\r\n", method, requestURI)
	hostHeader := parsed.Host
	if hostHeader == "" && host != "" {
		hostHeader = host
	}
	if hostHeader != "" {
		fmt.Fprintf(reqBuilder, "Host: %s\r\n", hostHeader)
	}
	reqBuilder.WriteString("User-Agent: hydr0g3n\r\n")
	reqBuilder.WriteString("Accept: */*\r\n")
	reqBuilder.WriteString("Connection: close\r\n\r\n")

	requestPayload := base64.StdEncoding.EncodeToString([]byte(reqBuilder.String()))

	status := res.StatusCode
	statusText := http.StatusText(status)
	if statusText == "" {
		statusText = "Status"
	}

	responseBody := res.Body
	responseLength := int(res.ContentLength)
	if responseLength < 0 {
		responseLength = len(responseBody)
	}

	respBuilder := &strings.Builder{}
	fmt.Fprintf(respBuilder, "HTTP/1.1 %d %s\r\n", status, statusText)
	fmt.Fprintf(respBuilder, "Content-Length: %d\r\n", len(responseBody))
	respBuilder.WriteString("Connection: close\r\n\r\n")
	if len(responseBody) > 0 {
		respBuilder.WriteString(string(responseBody))
	}

	responsePayload := base64.StdEncoding.EncodeToString([]byte(respBuilder.String()))

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

	if len(responseBody) > 0 || status != 0 {
		item.Response = burpMessage{Base64: "true", Value: responsePayload}
	}

	return item, nil
}
