package engine

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"hydr0g3n/pkg/httpclient"
	"hydr0g3n/pkg/store"
	"hydr0g3n/pkg/templater"
)

// Result captures the outcome of a single request executed by the engine.
type Result struct {
	URL           string
	StatusCode    int
	ContentLength int64
	Duration      time.Duration
	Body          []byte
	Err           error
}

// Config represents the parameters required to execute a fuzzing run.
type Config struct {
	URL             string
	Wordlist        string
	Concurrency     int
	Timeout         time.Duration
	OutputPath      string
	Profile         string
	Beginner        bool
	BinaryName      string
	RunRecorder     *store.Run
	Method          string
	FollowRedirects bool
}

// Run starts the fuzzing engine with the provided configuration. It launches a
// worker pool that performs concurrent HTTP requests using the configured method. The caller receives a
// channel of Result entries and is responsible for consuming it until closed.
func Run(ctx context.Context, cfg Config) (<-chan Result, error) {
	if cfg.URL == "" {
		return nil, errors.New("target URL is required")
	}

	if cfg.Wordlist == "" {
		return nil, errors.New("wordlist path is required")
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	file, err := os.Open(cfg.Wordlist)
	if err != nil {
		return nil, fmt.Errorf("open wordlist: %w", err)
	}

	jobs := make(chan string)
	results := make(chan Result)
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodHead
	}

	client := httpclient.New(timeout, cfg.FollowRedirects)

	tpl := templater.New()

	runRecorder := cfg.RunRecorder

	go func() {
		defer close(jobs)
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			word := strings.TrimSpace(scanner.Text())
			if word == "" {
				continue
			}

			url := tpl.Expand(cfg.URL, word)

			if runRecorder != nil {
				inserted, err := runRecorder.MarkAttempt(ctx, url)
				if err != nil {
					select {
					case <-ctx.Done():
						return
					case results <- Result{URL: url, Err: fmt.Errorf("record attempt: %w", err)}:
					}
					continue
				}

				if !inserted {
					continue
				}
			}

			select {
			case <-ctx.Done():
				return
			case jobs <- url:
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case <-ctx.Done():
			case results <- Result{Err: fmt.Errorf("read wordlist: %w", err)}:
			}
		}
	}()

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()

			for {
				select {
				case <-ctx.Done():
					return
				case url, ok := <-jobs:
					if !ok {
						return
					}

					res := executeRequest(ctx, client, url, timeout, method)

					select {
					case <-ctx.Done():
						return
					case results <- res:
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results, nil
}

func executeRequest(ctx context.Context, client *httpclient.Client, url string, timeout time.Duration, method string) Result {
	result := Result{URL: url}

	reqCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	start := time.Now()
	resp, err := client.Request(reqCtx, method, url)
	result.Duration = time.Since(start)
	if err != nil {
		result.Err = err
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	result.ContentLength = resp.ContentLength

	const maxBodyBytes = 1024 * 1024
	reader := io.LimitReader(resp.Body, maxBodyBytes)
	body, err := io.ReadAll(reader)
	if err != nil {
		result.Err = err
		return result
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	result.Body = body

	return result
}
