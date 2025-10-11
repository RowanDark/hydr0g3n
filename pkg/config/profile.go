package config

import (
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
