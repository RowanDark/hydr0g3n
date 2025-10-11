package engine

import "time"

// Config represents the parameters required to execute a fuzzing run.
type Config struct {
	URL         string
	Wordlist    string
	Concurrency int
	Timeout     time.Duration
	OutputPath  string
	Profile     string
	Beginner    bool
	BinaryName  string
}

// Run starts the fuzzing engine with the provided configuration.
// This is a placeholder implementation for the CLI bootstrap.
func Run(cfg Config) error {
	_ = cfg
	return nil
}
