package matcher

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"hydr0g3n/pkg/engine"
)

// Options defines the configuration for matching engine results.
type Options struct {
	Statuses            []int
	Size                SizeRange
	BaselineBody        []byte
	SimilarityThreshold float64
	ShingleSize         int
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
	statuses    map[int]struct{}
	hasStatus   bool
	size        SizeRange
	hasSizeAny  bool
	baseline    map[string]struct{}
	hasBaseline bool
	threshold   float64
	shingleSize int
}

// MatchOutcome describes the result of evaluating a response against the matcher rules.
type MatchOutcome struct {
	Matched       bool
	Similarity    float64
	HasSimilarity bool
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
	shingleSize := opts.ShingleSize
	if shingleSize <= 0 {
		shingleSize = 5
	}
	m.shingleSize = shingleSize
	if opts.SimilarityThreshold > 0 && len(opts.BaselineBody) > 0 {
		threshold := opts.SimilarityThreshold
		if threshold > 1 {
			threshold = 1
		}
		baseline := buildShingles(opts.BaselineBody, shingleSize)
		if len(baseline) > 0 {
			m.baseline = baseline
			m.threshold = threshold
			m.hasBaseline = true
		}
	}
	return m
}

// Matches returns true when the result passes all configured filters.
//
// Errors are always considered matches so they remain visible to the caller.
func (m Matcher) Matches(res engine.Result) bool {
	outcome := m.Evaluate(res)
	return outcome.Matched
}

// Evaluate determines whether the result passes all configured filters and returns
// additional metadata produced during evaluation.
func (m Matcher) Evaluate(res engine.Result) MatchOutcome {
	outcome := MatchOutcome{Matched: true}

	if res.Err != nil {
		return outcome
	}

	if m.hasStatus {
		if _, ok := m.statuses[res.StatusCode]; !ok {
			outcome.Matched = false
			return outcome
		}
	}

	if m.hasSizeAny {
		size := res.ContentLength
		if size < 0 {
			outcome.Matched = false
			return outcome
		}
		if m.size.HasMin && size < m.size.Min {
			outcome.Matched = false
			return outcome
		}
		if m.size.HasMax && size > m.size.Max {
			outcome.Matched = false
			return outcome
		}
	}

	if m.hasBaseline && m.threshold > 0 {
		if len(res.Body) == 0 {
			return outcome
		}
		shingles := buildShingles(res.Body, m.shingleSize)
		if len(shingles) == 0 {
			return outcome
		}
		similarity := jaccardSimilarity(m.baseline, shingles)
		outcome.Similarity = similarity
		outcome.HasSimilarity = true
		if similarity >= m.threshold {
			outcome.Matched = false
			return outcome
		}
	}

	return outcome
}

func buildShingles(body []byte, size int) map[string]struct{} {
	if size <= 0 {
		size = 1
	}
	tokens := tokenize(body)
	if len(tokens) == 0 {
		return nil
	}
	if len(tokens) < size {
		size = len(tokens)
	}
	if size <= 0 {
		return nil
	}
	shingles := make(map[string]struct{}, len(tokens))
	for i := 0; i <= len(tokens)-size; i++ {
		var builder strings.Builder
		for j := 0; j < size; j++ {
			if j > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteString(tokens[i+j])
		}
		shingles[builder.String()] = struct{}{}
	}
	return shingles
}

func tokenize(body []byte) []string {
	if len(body) == 0 {
		return nil
	}
	text := strings.ToLower(string(body))
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

func jaccardSimilarity(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for shingle := range b {
		if _, ok := a[shingle]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union <= 0 {
		return 0
	}
	return float64(intersection) / float64(union)
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
