package config

import (
	"fmt"
	"strings"
	"time"
)

// Profile describes a named set of runtime defaults for the engine.
type Profile struct {
	Method      string
	Concurrency int
	Throttle    time.Duration
	Recursive   bool
	Timeout     time.Duration
	Outputs     []string
}

var profiles = map[string]Profile{
	"beginner": {
		Method:      "HEAD",
		Concurrency: 10,
		Throttle:    100 * time.Millisecond,
		Recursive:   false,
		Timeout:     10 * time.Second,
		Outputs:     []string{"pretty", "jsonl"},
	},
}

// profileAliases maps aliases to their canonical profile names.
var profileAliases = map[string]string{
	"beginner": "beginner",
}

// LookupProfile returns the configuration for a named profile.
// The lookup is case-insensitive and follows profile aliases.
func LookupProfile(name string) (Profile, bool) {
	if name == "" {
		return Profile{}, false
	}

	canonical, ok := profileAliases[strings.ToLower(name)]
	if !ok {
		canonical = strings.ToLower(name)
	}

	profile, ok := profiles[canonical]
	return profile, ok
}

// RunHashConfig returns stable key/value entries that describe the profile for
// inclusion in run-hash calculations.
func (p Profile) RunHashConfig() []string {
	entries := []string{}

	if method := strings.ToUpper(strings.TrimSpace(p.Method)); method != "" {
		entries = append(entries, fmt.Sprintf("profile.method=%s", method))
	}
	if p.Concurrency > 0 {
		entries = append(entries, fmt.Sprintf("profile.concurrency=%d", p.Concurrency))
	}
	if p.Throttle > 0 {
		entries = append(entries, fmt.Sprintf("profile.throttle=%s", p.Throttle))
	}
	entries = append(entries, fmt.Sprintf("profile.recursive=%t", p.Recursive))
	if p.Timeout > 0 {
		entries = append(entries, fmt.Sprintf("profile.timeout=%s", p.Timeout))
	}
	if len(p.Outputs) > 0 {
		outputs := make([]string, 0, len(p.Outputs))
		for _, out := range p.Outputs {
			trimmed := strings.TrimSpace(out)
			if trimmed == "" {
				continue
			}
			outputs = append(outputs, trimmed)
		}
		if len(outputs) > 0 {
			entries = append(entries, fmt.Sprintf("profile.outputs=%s", strings.Join(outputs, ",")))
		}
	}

	return entries
}
