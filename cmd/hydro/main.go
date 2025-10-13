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

	"hydr0g3n/pkg/config"
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
		runID               = flag.String("run-id", "", "Override the deterministic run identifier used for persistence")
		followRedirects     = flag.Bool("follow-redirects", false, "Follow HTTP redirects (up to 5 hops)")
		similarityThreshold = flag.Float64("similarity-threshold", 0.6, "Hide hits whose bodies are this similar to the baseline (0-1)")
		noBaseline          = flag.Bool("no-baseline", false, "Disable the automatic baseline request used for similarity filtering")
		showSimilarity      = flag.Bool("show-similarity", false, "Include similarity scores in output (debug)")
		viewModeFlag        = flag.String("view", "table", "Pretty output layout (table, tree)")
		colorModeFlag       = flag.String("color-mode", "auto", "Color output mode (auto, always, never)")
		colorPresetFlag     = flag.String("color-preset", "default", "Color palette for pretty output (default, protanopia, tritanopia, blue-light)")
		burpExport          = flag.String("burp-export", "", "Write matched requests and responses to a Burp-compatible XML file")
		preHook             = flag.String("pre-hook", "", "Shell command to run once before requests to fetch auth headers (stdout JSON)")
		completionScript    = flag.String("completion-script", "", "Print shell completion script for the specified shell (bash, zsh, fish)")
		dryRun              = flag.Bool("dry-run", false, "Display planned permutations without sending any requests")
		progressFile        = flag.String("progress-file", "", "Path to store progress checkpoints for resuming runs")
	)

	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s -u <url> -w <wordlist> [options]\n", binaryName)
		fmt.Fprintln(flag.CommandLine.Output(), "\nFlags:")
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "\nExamples:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s --beginner -u https://example.com -w wordlists/common.txt\n", binaryName)
		fmt.Fprintln(flag.CommandLine.Output(), "\nFor detailed usage, install the man page and run: man hydro")
	}

	flag.Parse()

	viewMode, err := output.ParseViewMode(*viewModeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
		os.Exit(2)
	}

	colorMode, err := output.ParseColorMode(*colorModeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
		os.Exit(2)
	}

	colorPreset, err := output.ParseColorPreset(*colorPresetFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
		os.Exit(2)
	}

	if script := strings.TrimSpace(*completionScript); script != "" {
		if err := outputCompletionScript(os.Stdout, script); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
			os.Exit(2)
		}
		return
	}

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
	if !*noBaseline && !*dryRun {
		capturedBaseline, err := captureBaseline(ctx, *targetURL, *timeout, *followRedirects)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: baseline request failed: %v\n", binaryName, err)
		} else {
			baselineBody = capturedBaseline
		}
	}

	selectedProfile := *profile
	if *beginner {
		selectedProfile = "beginner"
	}

	binaryBase := filepath.Base(os.Args[0])

	runConfigEntries := []string{
		fmt.Sprintf("target_url=%s", strings.TrimSpace(*targetURL)),
		fmt.Sprintf("wordlist=%s", strings.TrimSpace(*wordlist)),
		fmt.Sprintf("method=%s", method),
		fmt.Sprintf("concurrency=%d", *concurrency),
		fmt.Sprintf("timeout=%s", timeout.String()),
		fmt.Sprintf("follow_redirects=%t", *followRedirects),
		fmt.Sprintf("similarity_threshold=%.6f", *similarityThreshold),
		fmt.Sprintf("no_baseline=%t", *noBaseline),
		fmt.Sprintf("beginner=%t", *beginner),
		fmt.Sprintf("binary=%s", binaryBase),
	}

	if *matchStatus != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("match_status=%s", strings.TrimSpace(*matchStatus)))
	}
	if *filterSize != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("filter_size=%s", strings.TrimSpace(*filterSize)))
	}
	if *outputPath != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("output_path=%s", *outputPath))
	}
	if *burpExport != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("burp_export=%s", *burpExport))
	}
	if *outputFormat != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("output_format=%s", strings.ToLower(*outputFormat)))
	}
	if *resumePath != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("resume_db=%s", *resumePath))
	}
	if trimmed := strings.TrimSpace(*progressFile); trimmed != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("progress_file=%s", trimmed))
	}
	if strings.TrimSpace(*preHook) != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("pre_hook=%s", strings.TrimSpace(*preHook)))
	}
	if selectedProfile != "" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("profile=%s", selectedProfile))
	}
	if viewValue := strings.ToLower(strings.TrimSpace(*viewModeFlag)); viewValue != "" && viewValue != "table" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("view=%s", viewValue))
	}
	if modeValue := strings.ToLower(strings.TrimSpace(*colorModeFlag)); modeValue != "" && modeValue != "auto" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("color_mode=%s", modeValue))
	}
	if presetValue := strings.ToLower(strings.TrimSpace(*colorPresetFlag)); presetValue != "" && presetValue != "default" {
		runConfigEntries = append(runConfigEntries, fmt.Sprintf("color_preset=%s", presetValue))
	}

	if prof, ok := config.LookupProfile(selectedProfile); ok {
		runConfigEntries = append(runConfigEntries, prof.RunHashConfig()...)
	}

	payloadEntries := []string{strings.TrimSpace(*wordlist)}

	runMeta := store.RunMetadata{
		TargetURL:   strings.TrimSpace(*targetURL),
		Wordlist:    strings.TrimSpace(*wordlist),
		Concurrency: *concurrency,
		Timeout:     *timeout,
		Profile:     selectedProfile,
		Beginner:    *beginner,
		BinaryName:  binaryBase,
		StartedAt:   time.Now().UTC(),
		RunID:       strings.TrimSpace(*runID),
		ConfigList:  runConfigEntries,
		PayloadList: payloadEntries,
	}

	if runMeta.RunID == "" {
		runMeta.RunID = runMeta.Hash()
	}

	runIdentifier := runMeta.RunID
	normalizedConfig := runMeta.ConfigEntries()
	normalizedPayloads := runMeta.PayloadEntries()

	var (
		resumeDB    *store.SQLite
		runRecorder *store.Run
	)

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
		PreHook:         strings.TrimSpace(*preHook),
		ProgressFile:    strings.TrimSpace(*progressFile),
	}

	if *dryRun {
		plan, err := engine.Plan(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: dry run failed: %v\n", binaryName, err)
			os.Exit(1)
		}

		fmt.Fprintf(os.Stdout, "Dry run: %d permutations", plan.TotalPermutations)
		if plan.QuickPermutations > 0 {
			fmt.Fprintf(os.Stdout, " (%d quick, %d primary)", plan.QuickPermutations, plan.PrimaryPermutations)
		}
		fmt.Fprintln(os.Stdout)

		if len(plan.Samples) > 0 {
			fmt.Fprintln(os.Stdout, "Samples:")
			for _, sample := range plan.Samples {
				fmt.Fprintf(os.Stdout, "  %s %s\n", method, sample)
			}
		} else {
			fmt.Fprintln(os.Stdout, "(no permutations generated)")
		}

		return
	}

	resultMatcher := matcher.New(matcher.Options{
		Statuses:            statuses,
		Size:                sizeRange,
		BaselineBody:        baselineBody,
		SimilarityThreshold: *similarityThreshold,
	})

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

		runRecorder, err = resumeDB.StartRun(ctx, runMeta)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
			os.Exit(1)
		}

		if stored := strings.TrimSpace(runRecorder.RunID()); stored != "" {
			runIdentifier = stored
		}
	}

	cfg.RunRecorder = runRecorder

	results, err := engine.Run(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
		os.Exit(1)
	}

	prettyWriter := output.NewPrettyWriter(os.Stdout, output.PrettyOptions{
		ShowSimilarity: *showSimilarity,
		ViewMode:       viewMode,
		ColorMode:      colorMode,
		ColorPreset:    colorPreset,
		TargetURL:      strings.TrimSpace(*targetURL),
	})

	var (
		jsonlWriter *output.JSONLWriter
		burpWriter  *output.BurpWriter
		writerErr   error
	)

	if *outputPath != "" {
		format := strings.ToLower(*outputFormat)
		switch format {
		case "jsonl", "":
			jsonlWriter, err = output.NewJSONLFile(*outputPath, *showSimilarity)
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

	if *burpExport != "" {
		burpWriter, err = output.NewBurpFile(*burpExport, method)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
			os.Exit(1)
		}
		defer func() {
			if closeErr := burpWriter.Close(); closeErr != nil && writerErr == nil {
				writerErr = closeErr
			}
		}()
	}

	if jsonlWriter != nil {
		header := output.RunHeader{
			RunID:     runIdentifier,
			TargetURL: runMeta.TargetURL,
			Wordlist:  runMeta.Wordlist,
			StartedAt: runMeta.StartedAt.Format(time.RFC3339Nano),
			Config:    normalizedConfig,
			Payloads:  normalizedPayloads,
		}

		if err := jsonlWriter.WriteHeader(header); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
			os.Exit(1)
		}
	}

	var runErr error

	for res := range results {
		outcome := resultMatcher.Evaluate(res)
		if outcome.HasSimilarity {
			res.HasSimilarity = true
			res.Similarity = outcome.Similarity
		}

		matches := outcome.Matched
		if matches {
			if jsonlWriter != nil {
				if err := jsonlWriter.Write(res); err != nil && writerErr == nil {
					writerErr = err
				}
			}
			if burpWriter != nil && res.Err == nil {
				if err := burpWriter.Write(res); err != nil && writerErr == nil {
					writerErr = err
				}
			}
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

		if !matches && jsonlWriter != nil {
			if err := jsonlWriter.Write(res); err != nil && writerErr == nil {
				writerErr = err
			}
		}

		if res.Err != nil && runErr == nil {
			runErr = res.Err
		}
	}

	if err := prettyWriter.Flush(); err != nil && writerErr == nil {
		writerErr = err
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

	resp, err := client.Request(reqCtx, http.MethodGet, url, nil)
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
