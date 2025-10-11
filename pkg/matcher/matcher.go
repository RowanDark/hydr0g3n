package matcher

import (
	"fmt"
	"strconv"
	"strings"

	"hydr0g3n/pkg/engine"
)

// Options defines the configuration for matching engine results.
type Options struct {
	Statuses []int
	Size     SizeRange
}

// SizeRange describes optional minimum and maximum bounds for the response size.
type SizeRange struct {
	Min    int64
	Max    int64
	HasMin bool
	HasMax bool
}

// Matcher evaluates engine results against a set of matching rules.
type Matcher struct {
	statuses   map[int]struct{}
	hasStatus  bool
	size       SizeRange
	hasSizeAny bool
}

// New creates a Matcher from the provided options.
func New(opts Options) Matcher {
	m := Matcher{size: opts.Size}
	if len(opts.Statuses) > 0 {
		m.statuses = make(map[int]struct{}, len(opts.Statuses))
		for _, code := range opts.Statuses {
			m.statuses[code] = struct{}{}
		}
		m.hasStatus = true
	}
	if opts.Size.HasMin || opts.Size.HasMax {
		m.hasSizeAny = true
	}
	return m
}

// Matches returns true when the result passes all configured filters.
//
// Errors are always considered matches so they remain visible to the caller.
func (m Matcher) Matches(res engine.Result) bool {
	if res.Err != nil {
		return true
	}

	if m.hasStatus {
		if _, ok := m.statuses[res.StatusCode]; !ok {
			return false
		}
	}

	if m.hasSizeAny {
		size := res.ContentLength
		if size < 0 {
			return false
		}
		if m.size.HasMin && size < m.size.Min {
			return false
		}
		if m.size.HasMax && size > m.size.Max {
			return false
		}
	}

	return true
}

// ParseStatusList converts a comma-separated list of HTTP status codes into integers.
func ParseStatusList(input string) ([]int, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}

	parts := strings.Split(input, ",")
	codes := make([]int, 0, len(parts))
	seen := make(map[int]struct{}, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, fmt.Errorf("empty status code in %q", input)
		}

		code, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid status code %q", trimmed)
		}

		if code < 100 || code > 999 {
			return nil, fmt.Errorf("status code out of range: %d", code)
		}

		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}

	return codes, nil
}

// ParseSizeRange parses a size range string in the form "min-max".
//
// The min or max values may be omitted to express open-ended ranges ("100-" or "-200").
func ParseSizeRange(input string) (SizeRange, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return SizeRange{}, nil
	}

	if strings.Count(input, "-") != 1 {
		return SizeRange{}, fmt.Errorf("invalid size range %q", input)
	}

	parts := strings.SplitN(input, "-", 2)
	var rng SizeRange

	if minStr := strings.TrimSpace(parts[0]); minStr != "" {
		min, err := strconv.ParseInt(minStr, 10, 64)
		if err != nil {
			return SizeRange{}, fmt.Errorf("invalid minimum size %q", minStr)
		}
		if min < 0 {
			return SizeRange{}, fmt.Errorf("minimum size must be non-negative: %d", min)
		}
		rng.Min = min
		rng.HasMin = true
	}

	if maxStr := strings.TrimSpace(parts[1]); maxStr != "" {
		max, err := strconv.ParseInt(maxStr, 10, 64)
		if err != nil {
			return SizeRange{}, fmt.Errorf("invalid maximum size %q", maxStr)
		}
		if max < 0 {
			return SizeRange{}, fmt.Errorf("maximum size must be non-negative: %d", max)
		}
		rng.Max = max
		rng.HasMax = true
	}

	if rng.HasMin && rng.HasMax && rng.Min > rng.Max {
		return SizeRange{}, fmt.Errorf("minimum size %d greater than maximum %d", rng.Min, rng.Max)
	}

	return rng, nil
}
