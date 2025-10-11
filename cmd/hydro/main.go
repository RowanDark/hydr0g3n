package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"hydr0g3n/pkg/engine"
)

func main() {
	const binaryName = "hydro"

	var (
		targetURL   = flag.String("u", "", "Target URL or template (required)")
		wordlist    = flag.String("w", "", "Path to the wordlist file (required)")
		concurrency = flag.Int("concurrency", 10, "Number of concurrent workers")
		timeout     = flag.Duration("timeout", 10*time.Second, "Request timeout duration")
		output      = flag.String("output", "", "Path to write output (JSONL or pretty)")
		beginner    = flag.Bool("beginner", false, "Enable beginner-friendly defaults")
		profile     = flag.String("profile", "", "Named execution profile to load")
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

	selectedProfile := *profile
	if *beginner {
		selectedProfile = "beginner"
	}

	cfg := engine.Config{
		URL:         *targetURL,
		Wordlist:    *wordlist,
		Concurrency: *concurrency,
		Timeout:     *timeout,
		OutputPath:  *output,
		Profile:     selectedProfile,
		Beginner:    *beginner,
		BinaryName:  filepath.Base(os.Args[0]),
	}

	if err := engine.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", binaryName, err)
		os.Exit(1)
	}
}

func exitWithUsage(message string) {
	fmt.Fprintf(os.Stderr, "Error: %s\n\n", message)
	flag.Usage()
	os.Exit(2)
}
