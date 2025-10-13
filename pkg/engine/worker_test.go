package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"hydr0g3n/pkg/httpclient"
	"hydr0g3n/pkg/templater"
)

func TestRunValidatesConfig(t *testing.T) {
	ctx := context.Background()

	if _, err := Run(ctx, Config{}); err == nil {
		t.Fatalf("expected error when URL is missing")
	}

	if _, err := Run(ctx, Config{URL: "https://example.com"}); err == nil {
		t.Fatalf("expected error when wordlist path is missing")
	}
}

func TestStageRunnerRunEmitsResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	dir := t.TempDir()
	wordlistPath := filepath.Join(dir, "wordlist.txt")
	if err := os.WriteFile(wordlistPath, []byte("admin\nuser\n"), 0o600); err != nil {
		t.Fatalf("write wordlist: %v", err)
	}

	client := httpclient.New(2*time.Second, false)
	resultsCh := make(chan Result, 8)
	runner := stageRunner{
		ctx:         ctx,
		target:      server.URL + "/FUZZ",
		concurrency: 2,
		timeout:     time.Second,
		method:      http.MethodGet,
		client:      client,
		tpl:         templater.New(),
		results:     resultsCh,
	}

	positive, err := runner.run(progressStagePrimary, wordlistPath, progressStageComplete, progressStageComplete)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	close(resultsCh)

	var results []Result
	for res := range resultsCh {
		results = append(results, res)
	}

	if !positive {
		t.Fatalf("expected quick positive detection")
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, res := range results {
		if res.Err != nil {
			t.Fatalf("unexpected error result: %v", res.Err)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code: %d", res.StatusCode)
		}
		if res.URL == runner.target {
			t.Fatalf("placeholder was not expanded in URL %q", res.URL)
		}
	}

	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected 2 requests, got %d", got)
	}
}
