package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// MatchEvent describes the information that is sent to an external plugin when
// a potential hit has been detected.
type MatchEvent struct {
	URL           string `json:"url"`
	Method        string `json:"method"`
	StatusCode    int    `json:"status_code"`
	ContentLength int64  `json:"content_length"`
	DurationMS    int64  `json:"duration_ms"`
	Body          []byte `json:"body,omitempty"`
	Error         string `json:"error,omitempty"`
}

// Response captures the values returned by the plugin.
type Response struct {
	Verify  *bool        `json:"verify,omitempty"`
	Request *RequestSpec `json:"request,omitempty"`
}

// RequestSpec contains optional overrides for issuing a follow-up HTTP request
// when a plugin asks hydr0g3n to perform additional verification.
type RequestSpec struct {
	URL             string            `json:"url,omitempty"`
	Method          string            `json:"method,omitempty"`
	Body            []byte            `json:"body,omitempty"`
	Headers         map[string]string `json:"headers,omitempty"`
	TimeoutMS       int64             `json:"timeout_ms,omitempty"`
	FollowRedirects *bool             `json:"follow_redirects,omitempty"`
}

// Call executes the plugin located at path and exchanges a JSON payload with
// it using stdin/stdout. The plugin receives the provided MatchEvent and is
// expected to emit a single JSON document describing its Response.
func Call(ctx context.Context, path string, event MatchEvent) (Response, error) {
	var resp Response

	if strings.TrimSpace(path) == "" {
		return resp, errors.New("plugin path is empty")
	}

	cmd := exec.CommandContext(ctx, path)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return resp, fmt.Errorf("open plugin stdin: %w", err)
	}
	defer stdin.Close()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return resp, fmt.Errorf("open plugin stdout: %w", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return resp, fmt.Errorf("start plugin: %w", err)
	}

	encoder := json.NewEncoder(stdin)
	if err := encoder.Encode(event); err != nil {
		cmd.Process.Kill()
		return resp, fmt.Errorf("encode plugin payload: %w", err)
	}
	if err := stdin.Close(); err != nil {
		cmd.Process.Kill()
		return resp, fmt.Errorf("close plugin stdin: %w", err)
	}

	output, err := io.ReadAll(stdout)
	if err != nil {
		cmd.Process.Kill()
		return resp, fmt.Errorf("read plugin stdout: %w", err)
	}

	if err := cmd.Wait(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return resp, fmt.Errorf("plugin error: %s: %w", errMsg, err)
		}
		return resp, fmt.Errorf("plugin error: %w", err)
	}

	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		if stderr.Len() > 0 {
			return resp, fmt.Errorf("plugin returned no output: %s", strings.TrimSpace(stderr.String()))
		}
		return resp, errors.New("plugin returned no output")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	if err := decoder.Decode(&resp); err != nil {
		return resp, fmt.Errorf("decode plugin response: %w", err)
	}

	if decoder.More() {
		return resp, errors.New("plugin produced extra output beyond a single JSON object")
	}

	return resp, nil
}
