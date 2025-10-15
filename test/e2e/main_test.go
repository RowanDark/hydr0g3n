package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"hydr0g3n/pkg/plugin"
)

var (
	hydroBinary string
	repoRoot    string
)

func TestMain(m *testing.M) {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: getwd: %v\n", err)
		os.Exit(1)
	}

	repoRoot = filepath.Dir(filepath.Dir(wd))

	buildDir, err := os.MkdirTemp("", "hydro-e2e-build-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: create build dir: %v\n", err)
		os.Exit(1)
	}

	hydroBinary = filepath.Join(buildDir, "hydro")

	cmd := exec.Command("go", "build", "-o", hydroBinary, "./cmd/hydro")
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")

	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build hydro: %v\n%s", err, output)
		os.Exit(1)
	}

	code := m.Run()

	_ = os.RemoveAll(buildDir)

	os.Exit(code)
}

type jsonlHeader struct {
	Type      string   `json:"type"`
	RunID     string   `json:"run_id"`
	TargetURL string   `json:"target_url"`
	Wordlist  string   `json:"wordlist"`
	Config    []string `json:"config"`
	Payloads  []string `json:"payloads"`
}

type jsonlEntry struct {
	URL       string  `json:"url"`
	Status    int     `json:"status"`
	Size      int64   `json:"size"`
	LatencyMS float64 `json:"latency_ms"`
	Error     string  `json:"error"`
}

func TestHydroEndToEndScanProducesJSONLAndSupportsPlugins(t *testing.T) {
	var (
		mu       sync.Mutex
		requests = make(map[string]int)
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case "/api/admin":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("admin success token"))
		case "/api/report":
			http.NotFound(w, r)
		case "/api/reports":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("reports index"))
		case "/api/status1":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("forbidden"))
		case "/api/status2":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("error"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()

	wordlist := strings.Join([]string{
		"admin",
		"report{,s}",
		"status[1-2]",
		"",
	}, "\n")
	wordlistPath := filepath.Join(dir, "wordlist.txt")
	if err := os.WriteFile(wordlistPath, []byte(wordlist), 0o600); err != nil {
		t.Fatalf("write wordlist: %v", err)
	}

	jsonlPath := filepath.Join(dir, "results.jsonl")

	_, _ = runHydroCommand(t,
		"-u", server.URL+"/api/FUZZ",
		"-w", wordlistPath,
		"--method", http.MethodGet,
		"--match-status", "200",
		"--output", jsonlPath,
		"--output-format", "jsonl",
		"--timeout", "2s",
		"--no-baseline",
		"--color-mode", "never",
	)

	header, entries := readJSONL(t, jsonlPath)

	if header.Type != "run" {
		t.Fatalf("unexpected header type: %q", header.Type)
	}
	if header.TargetURL == "" {
		t.Fatalf("expected target url in header")
	}
	if header.Wordlist != wordlistPath {
		t.Fatalf("unexpected wordlist in header: %q", header.Wordlist)
	}
	if len(header.Payloads) == 0 || header.Payloads[0] != wordlistPath {
		t.Fatalf("expected payload list to include wordlist path, got %v", header.Payloads)
	}

	if len(entries) != 5 {
		t.Fatalf("expected 5 result entries, got %d", len(entries))
	}

	mu.Lock()
	recorded := make(map[string]int, len(requests))
	for path, count := range requests {
		recorded[path] = count
	}
	mu.Unlock()

	expectedPaths := []string{
		"/api/admin",
		"/api/report",
		"/api/reports",
		"/api/status1",
		"/api/status2",
	}
	for _, path := range expectedPaths {
		if recorded[path] != 1 {
			t.Fatalf("expected request to %s exactly once, got %d", path, recorded[path])
		}
	}

	var adminEntry *jsonlEntry
	for i := range entries {
		entry := &entries[i]
		if strings.HasSuffix(entry.URL, "/api/admin") {
			adminEntry = entry
			break
		}
	}
	if adminEntry == nil {
		t.Fatalf("admin entry not found in results: %+v", entries)
	}
	if adminEntry.Status != http.StatusOK {
		t.Fatalf("unexpected admin status: %d", adminEntry.Status)
	}
	if adminEntry.Size <= 0 {
		t.Fatalf("expected positive content length for admin entry, got %d", adminEntry.Size)
	}

	pluginPath := writeVerifierPlugin(t, dir, "success token")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := plugin.Call(ctx, pluginPath, plugin.MatchEvent{
		URL:           adminEntry.URL,
		Method:        http.MethodGet,
		StatusCode:    adminEntry.Status,
		ContentLength: adminEntry.Size,
		DurationMS:    int64(adminEntry.LatencyMS),
	})
	if err != nil {
		t.Fatalf("plugin call failed: %v", err)
	}
	if resp.Verify == nil || !*resp.Verify {
		t.Fatalf("expected plugin verification success, got %+v", resp)
	}
}

func TestHydroResumeSkipsCompletedRequests(t *testing.T) {
	var (
		mu       sync.Mutex
		requests = make(map[string]int)
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests[r.URL.Path]++
		mu.Unlock()

		switch r.URL.Path {
		case "/api/alpha", "/api/beta":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()

	wordlistPath := filepath.Join(dir, "words.txt")
	if err := os.WriteFile(wordlistPath, []byte("alpha\nbeta\n"), 0o600); err != nil {
		t.Fatalf("write wordlist: %v", err)
	}

	resumeDB := filepath.Join(dir, "resume.db")

	args := []string{
		"-u", server.URL + "/api/FUZZ",
		"-w", wordlistPath,
		"--method", http.MethodGet,
		"--match-status", "200",
		"--no-baseline",
		"--timeout", "2s",
		"--resume", resumeDB,
		"--run-id", "e2e-resume",
		"--concurrency", "1",
		"--color-mode", "never",
	}

	_, _ = runHydroCommand(t, args...)

	mu.Lock()
	first := copyMap(requests)
	mu.Unlock()

	for _, path := range []string{"/api/alpha", "/api/beta"} {
		if first[path] != 1 {
			t.Fatalf("expected %s to be requested once on initial run, got %d", path, first[path])
		}
	}

	_, _ = runHydroCommand(t, args...)

	mu.Lock()
	second := copyMap(requests)
	mu.Unlock()

	for _, path := range []string{"/api/alpha", "/api/beta"} {
		if second[path] != first[path] {
			t.Fatalf("expected %s count to remain %d after resume, got %d", path, first[path], second[path])
		}
	}
}

func runHydroCommand(t *testing.T, args ...string) (string, string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, hydroBinary, args...)
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			t.Fatalf("hydro command timed out; stdout=%s stderr=%s", stdout.String(), stderr.String())
		}
		t.Fatalf("hydro command failed: %v\nstdout:%s\nstderr:%s", err, stdout.String(), stderr.String())
	}

	return stdout.String(), stderr.String()
}

func readJSONL(t *testing.T, path string) (jsonlHeader, []jsonlEntry) {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open jsonl: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 1024), 1024*1024)

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			t.Fatalf("read header: %v", err)
		}
		t.Fatalf("jsonl file %s is empty", path)
	}

	var header jsonlHeader
	if err := json.Unmarshal(scanner.Bytes(), &header); err != nil {
		t.Fatalf("decode header: %v", err)
	}

	var entries []jsonlEntry
	for scanner.Scan() {
		var entry jsonlEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("decode entry: %v", err)
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan jsonl: %v", err)
	}

	return header, entries
}

func writeVerifierPlugin(t *testing.T, dir, token string) string {
	t.Helper()

	script := fmt.Sprintf(`#!/usr/bin/env python3
import json
import sys
import urllib.request


def main():
    event = json.load(sys.stdin)
    url = event.get("url")
    if not url:
        print("missing url", file=sys.stderr)
        sys.exit(1)
    with urllib.request.urlopen(url, timeout=5) as response:
        body = response.read().decode("utf-8", errors="ignore")
    result = %q in body
    json.dump({"verify": result}, sys.stdout)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
`, token)

	path := filepath.Join(dir, "verifier.py")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	return path
}

func copyMap(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
