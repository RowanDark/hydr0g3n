package engine

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hydr0g3n/bench"
	"hydr0g3n/pkg/httpclient"
	"hydr0g3n/pkg/templater"
)

const requestsPerIteration = 64

func BenchmarkStageRunner(b *testing.B) {
	srv := bench.NewServer()
	b.Cleanup(func() {
		srv.Close()
	})

	target := srv.URL() + "/FUZZ"
	dir := b.TempDir()

	cases := []struct {
		name string
		word string
	}{
		{name: "fast", word: "fast"},
		{name: "slow", word: "slow"},
		{name: "notfound", word: "ghost"},
	}

	concLevels := []int{1, 4, 16}

	for _, bc := range cases {
		bc := bc
		wordlist := buildWordlist(b, dir, bc.name, bc.word, requestsPerIteration)

		b.Run(bc.name, func(b *testing.B) {
			for _, conc := range concLevels {
				conc := conc
				b.Run(fmt.Sprintf("c%d", conc), func(b *testing.B) {
					client := httpclient.New(5*time.Second, false)
					tpl := templater.New()

					benchmarkStageRunner(b, target, wordlist, conc, client, tpl)
				})
			}
		})
	}
}

func benchmarkStageRunner(b *testing.B, target, wordlist string, concurrency int, client *httpclient.Client, tpl *templater.Templater) {
	b.Helper()

	ctx := context.Background()

	runOnce := func() {
		resultsCh := make(chan Result, requestsPerIteration)
		runner := stageRunner{
			ctx:         ctx,
			target:      target,
			concurrency: concurrency,
			timeout:     time.Second,
			method:      http.MethodGet,
			client:      client,
			tpl:         tpl,
			results:     resultsCh,
		}

		drained := make(chan struct{})
		go func() {
			for range resultsCh {
			}
			close(drained)
		}()

		if _, err := runner.run("bench", wordlist, "", ""); err != nil {
			b.Fatalf("run: %v", err)
		}

		close(resultsCh)
		<-drained
	}

	b.ResetTimer()
	start := time.Now()
	for i := 0; i < b.N; i++ {
		runOnce()
	}
	elapsed := time.Since(start)
	b.StopTimer()

	totalRequests := float64(b.N * requestsPerIteration)
	if elapsed > 0 {
		// Convert to requests per second based on the total requests issued.
		b.ReportMetric(totalRequests/elapsed.Seconds(), "req/s")
	}
	b.ReportMetric(float64(requestsPerIteration), "requests/op")
}

func buildWordlist(tb testing.TB, dir, name, word string, count int) string {
	tb.Helper()

	path := filepath.Join(dir, fmt.Sprintf("%s.txt", name))
	lines := strings.Repeat(word+"\n", count)
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		tb.Fatalf("write wordlist: %v", err)
	}
	return path
}
