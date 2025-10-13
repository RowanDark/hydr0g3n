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
	ProgressFile    string
}

// PlanSummary describes the permutations that would be executed for a given
// configuration without issuing any network requests.
type PlanSummary struct {
	QuickPermutations   int
	PrimaryPermutations int
	TotalPermutations   int
	Samples             []string
}

const planSampleLimit = 10

// Plan enumerates the permutations for the provided configuration and returns
// a summary containing counts and representative samples.
func Plan(cfg Config) (*PlanSummary, error) {
	if cfg.URL == "" {
		return nil, errors.New("target URL is required")
	}

	if cfg.Wordlist == "" {
		return nil, errors.New("wordlist path is required")
	}

	tpl := templater.New()
	samples := make([]string, 0, planSampleLimit)
	addSample := func(url string) {
		if len(samples) < planSampleLimit {
			samples = append(samples, url)
		}
	}

	summary := &PlanSummary{}

	quickEnabled := cfg.Quick || cfg.Beginner
	quickWordlist := ""
	if quickEnabled {
		quickWordlist = locateQuickWordlist(cfg.Wordlist)
		if quickWordlist == "" {
			quickEnabled = false
		}
	}

	if quickEnabled {
		count, err := countWordlistPermutations(quickWordlist, cfg.URL, tpl, addSample)
		if err != nil {
			return nil, err
		}
		summary.QuickPermutations = count
		summary.TotalPermutations += count
	}

	primaryCount, err := countWordlistPermutations(cfg.Wordlist, cfg.URL, tpl, addSample)
	if err != nil {
		return nil, err
	}

	summary.PrimaryPermutations = primaryCount
	summary.TotalPermutations += primaryCount
	summary.Samples = samples

	return summary, nil
}

func countWordlistPermutations(path, target string, tpl *templater.Templater, addSample func(string)) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open wordlist: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	total := 0

	for scanner.Scan() {
		word := strings.TrimSpace(scanner.Text())
		if word == "" {
			continue
		}

		payloads := tpl.ExpandPayload(word)
		for _, payload := range payloads {
			total++
			if addSample != nil {
				addSample(tpl.Expand(target, payload))
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("read wordlist: %w", err)
	}

	return total, nil
}

const (
	progressStageQuick    = "quick"
	progressStagePrimary  = "primary"
	progressStageComplete = "complete"
)

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

	progressTracker, err := newProgressTracker(strings.TrimSpace(cfg.ProgressFile))
	if err != nil {
		return nil, err
	}

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
			progress:    progressTracker,
		}

		if quickEnabled {
			positive, err := runner.run(progressStageQuick, quickWordlist, progressStagePrimary, progressStageComplete)
			if err != nil {
				runner.emit(Result{Err: err})
				return
			}

			if !positive {
				return
			}
		}

		if _, err := runner.run(progressStagePrimary, cfg.Wordlist, progressStageComplete, progressStageComplete); err != nil {
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
	progress    *progressTracker
}

func (r *stageRunner) run(stage string, wordlistPath string, nextStageOnSuccess, nextStageOnFailure string) (bool, error) {
	if r.progress != nil {
		if err := r.progress.EnsureStage(stage); err != nil {
			return false, err
		}

		if r.progress.StageCompleted(stage) {
			if stage == progressStageQuick {
				state := r.progress.State()
				if state.Stage == progressStagePrimary {
					return true, nil
				}
			}
			return false, nil
		}
	}

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
	wordIndex := 0

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
		for variantIndex, payload := range payloads {
			if r.progress != nil && !r.progress.Allow(stage, wordIndex, variantIndex) {
				continue
			}

			url := r.tpl.Expand(r.target, payload)

			nextWord := wordIndex
			nextVariant := variantIndex + 1
			if nextVariant >= len(payloads) {
				nextWord = wordIndex + 1
				nextVariant = 0
			}

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
					if !r.updateProgress(stage, nextWord, nextVariant, url) {
						stop = true
						break
					}
					continue
				}
			}

			if !r.enqueue(jobs, url) {
				stop = true
				break
			}

			if !r.updateProgress(stage, nextWord, nextVariant, url) {
				stop = true
				break
			}
		}

		if stop {
			break
		}

		wordIndex++
	}

	if err := scanner.Err(); err != nil && !stop {
		r.emit(Result{Err: fmt.Errorf("read wordlist: %w", err)})
	}

	close(jobs)
	wg.Wait()

	completed := !stop
	positiveResult := positive.Load()

	if completed && r.progress != nil {
		nextStage := nextStageOnFailure
		if positiveResult {
			nextStage = nextStageOnSuccess
		}

		if nextStage != "" {
			if err := r.progress.Set(nextStage, 0, 0); err != nil {
				return positiveResult, err
			}
		}
	}

	return positiveResult, nil
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

func (r *stageRunner) updateProgress(stage string, wordIndex, variantIndex int, url string) bool {
	if r.progress == nil {
		return true
	}

	if err := r.progress.Set(stage, wordIndex, variantIndex); err != nil {
		r.emit(Result{URL: url, Err: fmt.Errorf("write progress: %w", err)})
		return false
	}

	return true
}

type progressState struct {
	Stage        string `json:"stage"`
	WordIndex    int    `json:"word_index"`
	VariantIndex int    `json:"variant_index"`
}

type progressTracker struct {
	path     string
	mu       sync.Mutex
	state    progressState
	hasState bool
}

func newProgressTracker(path string) (*progressTracker, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}

	tracker := &progressTracker{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return tracker, nil
		}
		return nil, fmt.Errorf("read progress file: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return tracker, nil
	}

	if err := json.Unmarshal(data, &tracker.state); err != nil {
		return nil, fmt.Errorf("decode progress file: %w", err)
	}

	tracker.hasState = true

	return tracker, nil
}

func (p *progressTracker) EnsureStage(stage string) error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.hasState && stageRank(stage) <= stageRank(p.state.Stage) {
		return nil
	}

	p.state = progressState{Stage: stage}
	p.hasState = true

	return p.writeLocked()
}

func (p *progressTracker) StageCompleted(stage string) bool {
	if p == nil {
		return false
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.hasState {
		return false
	}

	return stageRank(stage) < stageRank(p.state.Stage)
}

func (p *progressTracker) Allow(stage string, wordIndex, variantIndex int) bool {
	if p == nil {
		return true
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.hasState {
		return true
	}

	currentStage := stageRank(stage)
	storedStage := stageRank(p.state.Stage)

	if currentStage < storedStage {
		return false
	}
	if currentStage > storedStage {
		return true
	}

	if wordIndex < p.state.WordIndex {
		return false
	}
	if wordIndex > p.state.WordIndex {
		return true
	}

	return variantIndex >= p.state.VariantIndex
}

func (p *progressTracker) Set(stage string, wordIndex, variantIndex int) error {
	if p == nil {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.state = progressState{
		Stage:        stage,
		WordIndex:    wordIndex,
		VariantIndex: variantIndex,
	}
	p.hasState = true

	return p.writeLocked()
}

func (p *progressTracker) State() progressState {
	if p == nil {
		return progressState{}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	return p.state
}

func (p *progressTracker) writeLocked() error {
	if p == nil {
		return nil
	}

	if err := ensureProgressDir(p.path); err != nil {
		return err
	}

	dir := filepath.Dir(p.path)
	tmp, err := os.CreateTemp(dir, "progress-*.tmp")
	if err != nil {
		return fmt.Errorf("create progress temp file: %w", err)
	}

	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(p.state); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return fmt.Errorf("encode progress checkpoint: %w", err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("close progress temp file: %w", err)
	}

	if err := os.Rename(tmp.Name(), p.path); err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("replace progress file: %w", err)
	}

	return nil
}

func ensureProgressDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "." || dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create progress directory: %w", err)
	}

	return nil
}

func stageRank(stage string) int {
	switch stage {
	case progressStageQuick:
		return 0
	case progressStagePrimary:
		return 1
	case progressStageComplete:
		return 2
	default:
		return -1
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
