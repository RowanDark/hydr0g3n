package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hydr0g3n/pkg/engine"
	"hydr0g3n/pkg/httpclient"
	"hydr0g3n/pkg/matcher"
	"hydr0g3n/pkg/output"
	"hydr0g3n/pkg/store"
	"hydr0g3n/pkg/templater"
)

func main() {
	const binaryName = "hydro"

	var (
		targetURL           = flag.String("u", "", "Target URL or template (required)")
		wordlist            = flag.String("w", "", "Path to the wordlist file (required)")
		concurrency         = flag.Int("concurrency", 10, "Number of concurrent workers")
		timeout             = flag.Duration("timeout", 10*time.Second, "Request timeout duration")
		outputPath          = flag.String("output", "", "Path to write output results")
		outputFormat        = flag.String("output-format", "jsonl", "Format for --output (jsonl)")
		beginner            = flag.Bool("beginner", false, "Enable beginner-friendly defaults")
		profile             = flag.String("profile", "", "Named execution profile to load")
		matchStatus         = flag.String("match-status", "", "Comma-separated list of HTTP status codes to include in hits")
		filterSize          = flag.String("filter-size", "", "Filter visible hits by response size range (min-max bytes)")
		resumePath          = flag.String("resume", "", "Path to a SQLite database for resuming and recording runs")
		methodFlag          = flag.String("method", http.MethodHead, "HTTP method to use for requests (GET, HEAD, POST)")
		followRedirects     = flag.Bool("follow-redirects", false, "Follow HTTP redirects (up to 5 hops)")
		similarityThreshold = flag.Float64("similarity-threshold", 0.6, "Hide hits whose bodies are this similar to the baseline (0-1)")
		noBaseline          = flag.Bool("no-baseline", false, "Disable the automatic baseline request used for similarity filtering")
	)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s -u <url> -w <wordlist> [options]\n", binaryName)
		fmt.Fprintln(flag.CommandLine.Output(), "\nFlags:")
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "\nExamples:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s --beginner -u https://example.com -w wordlists/common.txt\n", binaryName)
	}

	flag.Parse()

	if *targetURL == "" {
		exitWithUsage("a target URL must be provided with -u")
	}

	if *wordlist == "" {
		exitWithUsage("a wordlist must be provided with -w")
	}

	method := strings.ToUpper(strings.TrimSpace(*methodFlag))
	if method == "" {
		method = http.MethodHead
	}

	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost:
	default:
		fmt.Fprintf(os.Stderr, "%s: unsupported HTTP method %q\n", binaryName, method)
		os.Exit(2)
	}

	statuses, err := matcher.ParseStatusList(*matchStatus)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
		os.Exit(2)
	}

	sizeRange, err := matcher.ParseSizeRange(*filterSize)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
		os.Exit(2)
	}

	if *similarityThreshold < 0 || *similarityThreshold > 1 {
		fmt.Fprintf(os.Stderr, "%s: --similarity-threshold must be between 0 and 1\n", binaryName)
		os.Exit(2)
	}

	ctx := context.Background()

	var baselineBody []byte
	if !*noBaseline {
		capturedBaseline, err := captureBaseline(ctx, *targetURL, *timeout, *followRedirects)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: baseline request failed: %v\n", binaryName, err)
		} else {
			baselineBody = capturedBaseline
		}
	}

	resultMatcher := matcher.New(matcher.Options{
		Statuses:            statuses,
		Size:                sizeRange,
		BaselineBody:        baselineBody,
		SimilarityThreshold: *similarityThreshold,
	})

	selectedProfile := *profile
	if *beginner {
		selectedProfile = "beginner"
	}

	binaryBase := filepath.Base(os.Args[0])

	var (
		resumeDB    *store.SQLite
		runRecorder *store.Run
	)

	if *resumePath != "" {
		var err error
		resumeDB, err = store.OpenSQLite(*resumePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
			os.Exit(1)
		}

		defer func() {
			if err := resumeDB.Close(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: close resume db: %v\n", binaryName, err)
			}
		}()

		runRecorder, err = resumeDB.StartRun(ctx, store.RunMetadata{
			TargetURL:   *targetURL,
			Wordlist:    *wordlist,
			Concurrency: *concurrency,
			Timeout:     *timeout,
			Profile:     selectedProfile,
			Beginner:    *beginner,
			BinaryName:  binaryBase,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
			os.Exit(1)
		}
	}

	cfg := engine.Config{
		URL:             *targetURL,
		Wordlist:        *wordlist,
		Concurrency:     *concurrency,
		Timeout:         *timeout,
		OutputPath:      *outputPath,
		Profile:         selectedProfile,
		Beginner:        *beginner,
		BinaryName:      binaryBase,
		RunRecorder:     runRecorder,
		Method:          method,
		FollowRedirects: *followRedirects,
	}

	results, err := engine.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
		os.Exit(1)
	}

	prettyWriter := output.NewPrettyWriter(os.Stdout)

	var (
		jsonlWriter *output.JSONLWriter
		writerErr   error
	)

	if *outputPath != "" {
		format := strings.ToLower(*outputFormat)
		switch format {
		case "jsonl", "":
			jsonlWriter, err = output.NewJSONLFile(*outputPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
				os.Exit(1)
			}
			defer func() {
				if closeErr := jsonlWriter.Close(); closeErr != nil && writerErr == nil {
					writerErr = closeErr
				}
			}()
		default:
			fmt.Fprintf(os.Stderr, "%s: unsupported output format %q\n", binaryName, format)
			os.Exit(2)
		}
	}

	var runErr error

	for res := range results {
		if jsonlWriter != nil {
			if err := jsonlWriter.Write(res); err != nil && writerErr == nil {
				writerErr = err
			}
		}

		matches := resultMatcher.Matches(res)
		if matches {
			if runRecorder != nil {
				if err := runRecorder.RecordHit(ctx, store.HitRecord{
					Path:          res.URL,
					StatusCode:    res.StatusCode,
					ContentLength: res.ContentLength,
					Duration:      res.Duration,
				}); err != nil && writerErr == nil {
					writerErr = err
				}
			}

			if err := prettyWriter.Write(res); err != nil && writerErr == nil {
				writerErr = err
			}
		}

		if res.Err != nil && runErr == nil {
			runErr = res.Err
		}
	}

	if writerErr != nil {
		fmt.Fprintf(os.Stderr, "%s: output error: %v\n", binaryName, writerErr)
		os.Exit(1)
	}

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, runErr)
		os.Exit(1)
	}
}

func exitWithUsage(message string) {
	fmt.Fprintf(os.Stderr, "Error: %s\n\n", message)
	flag.Usage()
	os.Exit(2)
}

func captureBaseline(ctx context.Context, target string, timeout time.Duration, followRedirects bool) ([]byte, error) {
	client := httpclient.New(timeout, followRedirects)
	tpl := templater.New()
	url := tpl.Expand(target, randomToken())

	reqCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	resp, err := client.Request(reqCtx, http.MethodGet, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	const maxBaselineBytes = 1024 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBaselineBytes))
	if err != nil {
		return nil, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	return body, nil
}

func randomToken() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("baseline-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
