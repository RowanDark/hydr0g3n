package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hydr0g3n/pkg/engine"
	"hydr0g3n/pkg/matcher"
	"hydr0g3n/pkg/output"
)

func main() {
	const binaryName = "hydro"

	var (
		targetURL    = flag.String("u", "", "Target URL or template (required)")
		wordlist     = flag.String("w", "", "Path to the wordlist file (required)")
		concurrency  = flag.Int("concurrency", 10, "Number of concurrent workers")
		timeout      = flag.Duration("timeout", 10*time.Second, "Request timeout duration")
		outputPath   = flag.String("output", "", "Path to write output results")
		outputFormat = flag.String("output-format", "jsonl", "Format for --output (jsonl)")
		beginner     = flag.Bool("beginner", false, "Enable beginner-friendly defaults")
		profile      = flag.String("profile", "", "Named execution profile to load")
		matchStatus  = flag.String("match-status", "", "Comma-separated list of HTTP status codes to include in hits")
		filterSize   = flag.String("filter-size", "", "Filter visible hits by response size range (min-max bytes)")
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

	resultMatcher := matcher.New(matcher.Options{Statuses: statuses, Size: sizeRange})

	selectedProfile := *profile
	if *beginner {
		selectedProfile = "beginner"
	}

	cfg := engine.Config{
		URL:         *targetURL,
		Wordlist:    *wordlist,
		Concurrency: *concurrency,
		Timeout:     *timeout,
		OutputPath:  *outputPath,
		Profile:     selectedProfile,
		Beginner:    *beginner,
		BinaryName:  filepath.Base(os.Args[0]),
	}

	ctx := context.Background()
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

		if resultMatcher.Matches(res) {
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
