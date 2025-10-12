package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	Similarity    float64
	HasSimilarity bool
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
	Quick           bool
	BinaryName      string
	RunRecorder     *store.Run
	Method          string
	FollowRedirects bool
	PreHook         string
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

	if file, err := os.Open(cfg.Wordlist); err != nil {
		return nil, fmt.Errorf("open wordlist: %w", err)
	} else {
		file.Close()
	}

	results := make(chan Result)
	method := strings.ToUpper(cfg.Method)
	if method == "" {
		method = http.MethodHead
	}

	client := httpclient.New(timeout, cfg.FollowRedirects)

	tpl := templater.New()

	runRecorder := cfg.RunRecorder

	quickEnabled := cfg.Quick || cfg.Beginner
	quickWordlist := ""
	if quickEnabled {
		quickWordlist = locateQuickWordlist(cfg.Wordlist)
		if quickWordlist == "" {
			quickEnabled = false
		}
	}

	requestOpts, err := runPreHook(ctx, cfg.PreHook)
	if err != nil {
		return nil, err
	}

	go func() {
		defer close(results)

		runner := stageRunner{
			ctx:         ctx,
			target:      cfg.URL,
			concurrency: concurrency,
			timeout:     timeout,
			method:      method,
			client:      client,
			tpl:         tpl,
			runRecorder: runRecorder,
			results:     results,
			requestOpts: requestOpts,
		}

		if quickEnabled {
			positive, err := runner.run(quickWordlist)
			if err != nil {
				runner.emit(Result{Err: err})
				return
			}

			if !positive {
				return
			}
		}

		if _, err := runner.run(cfg.Wordlist); err != nil {
			runner.emit(Result{Err: err})
		}
	}()

	return results, nil
}

func executeRequest(ctx context.Context, client *httpclient.Client, url string, timeout time.Duration, method string, opts *httpclient.RequestOptions) Result {
	result := Result{URL: url}

	reqCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	start := time.Now()
	resp, err := client.Request(reqCtx, method, url, opts)
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

type stageRunner struct {
	ctx         context.Context
	target      string
	concurrency int
	timeout     time.Duration
	method      string
	client      *httpclient.Client
	tpl         *templater.Templater
	runRecorder *store.Run
	results     chan<- Result
	requestOpts *httpclient.RequestOptions
}

func (r *stageRunner) run(wordlistPath string) (bool, error) {
	file, err := os.Open(wordlistPath)
	if err != nil {
		return false, fmt.Errorf("open wordlist: %w", err)
	}
	defer file.Close()

	jobs := make(chan string)
	var wg sync.WaitGroup
	var positive atomic.Bool

	worker := func() {
		defer wg.Done()

		for {
			select {
			case <-r.ctx.Done():
				return
			case url, ok := <-jobs:
				if !ok {
					return
				}

				res := executeRequest(r.ctx, r.client, url, r.timeout, r.method, r.requestOpts)

				if res.Err == nil && isQuickPositive(res.StatusCode) {
					positive.Store(true)
				}

				if !r.emit(res) {
					return
				}
			}
		}
	}

	wg.Add(r.concurrency)
	for i := 0; i < r.concurrency; i++ {
		go worker()
	}

	scanner := bufio.NewScanner(file)
	stop := false

	for scanner.Scan() {
		if r.ctx.Err() != nil {
			stop = true
			break
		}

		word := strings.TrimSpace(scanner.Text())
		if word == "" {
			continue
		}

		payloads := r.tpl.ExpandPayload(word)
		for _, payload := range payloads {
			url := r.tpl.Expand(r.target, payload)

			if r.runRecorder != nil {
				inserted, err := r.runRecorder.MarkAttempt(r.ctx, url)
				if err != nil {
					if !r.emit(Result{URL: url, Err: fmt.Errorf("record attempt: %w", err)}) {
						stop = true
						break
					}
					continue
				}

				if !inserted {
					continue
				}
			}

			if !r.enqueue(jobs, url) {
				stop = true
				break
			}
		}

		if stop {
			break
		}
	}

	if err := scanner.Err(); err != nil && !stop {
		r.emit(Result{Err: fmt.Errorf("read wordlist: %w", err)})
	}

	close(jobs)
	wg.Wait()

	return positive.Load(), nil
}

func (r *stageRunner) emit(res Result) bool {
	select {
	case <-r.ctx.Done():
		return false
	case r.results <- res:
		return true
	}
}

func (r *stageRunner) enqueue(jobs chan<- string, url string) bool {
	select {
	case <-r.ctx.Done():
		return false
	case jobs <- url:
		return true
	}
}

type preHookResponse struct {
	Cookie  string            `json:"cookie"`
	Headers map[string]string `json:"headers"`
}

func runPreHook(ctx context.Context, command string) (*httpclient.RequestOptions, error) {
	if strings.TrimSpace(command) == "" {
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("pre-hook: %w", err)
	}

	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, errors.New("pre-hook: empty output")
	}

	var parsed preHookResponse
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		return nil, fmt.Errorf("pre-hook: decode output: %w", err)
	}

	opts := &httpclient.RequestOptions{}

	if parsed.Cookie != "" {
		opts.Cookie = parsed.Cookie
	}

	if len(parsed.Headers) > 0 {
		headers := make(http.Header, len(parsed.Headers))
		for key, value := range parsed.Headers {
			if strings.TrimSpace(key) == "" {
				continue
			}
			headers.Set(key, value)
		}
		opts.Headers = headers
	}

	if opts.Cookie == "" && len(opts.Headers) == 0 {
		return nil, nil
	}

	return opts, nil
}

func locateQuickWordlist(primary string) string {
	candidates := []string{}

	if primary != "" {
		dir := filepath.Dir(primary)
		if dir == "." || dir == "" {
			candidates = append(candidates, "sample_small.txt")
		} else {
			candidates = append(candidates, filepath.Join(dir, "sample_small.txt"))
		}
	}

	candidates = append(candidates, filepath.Join("wordlists", "sample_small.txt"))

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}

		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}

		return candidate
	}

	return ""
}

func isQuickPositive(status int) bool {
	if status == 0 {
		return false
	}

	if status >= 200 && status < 400 {
		return true
	}

	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusMethodNotAllowed:
		return true
	default:
		return false
	}
}
