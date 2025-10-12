package matcher

import (
	"errors"
	"testing"

	"hydr0g3n/pkg/engine"
)

func TestParseStatusList(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{name: "empty", input: "", want: nil},
		{name: "single", input: "200", want: []int{200}},
		{name: "multiple", input: "200,301", want: []int{200, 301}},
		{name: "spaces", input: "200, 404", want: []int{200, 404}},
		{name: "duplicate", input: "200,200", want: []int{200}},
		{name: "invalid", input: "abc", wantErr: true},
		{name: "out of range", input: "42", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseStatusList(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("length mismatch: got %d want %d", len(got), len(tt.want))
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("at %d got %d want %d", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParseSizeRange(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    SizeRange
		wantErr bool
	}{
		{name: "empty", input: "", want: SizeRange{}},
		{name: "min max", input: "10-20", want: SizeRange{Min: 10, Max: 20, HasMin: true, HasMax: true}},
		{name: "open max", input: "100-", want: SizeRange{Min: 100, HasMin: true}},
		{name: "open min", input: "-200", want: SizeRange{Max: 200, HasMax: true}},
		{name: "invalid format", input: "100", wantErr: true},
		{name: "min greater than max", input: "20-10", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSizeRange(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestMatcherMatches(t *testing.T) {
	opts := Options{
		Statuses: []int{200, 301},
		Size: SizeRange{
			Min:    100,
			Max:    500,
			HasMin: true,
			HasMax: true,
		},
	}
	matcher := New(opts)

	tests := []struct {
		name string
		res  engine.Result
		want bool
	}{
		{name: "match", res: engine.Result{StatusCode: 200, ContentLength: 200}, want: true},
		{name: "status mismatch", res: engine.Result{StatusCode: 404, ContentLength: 200}, want: false},
		{name: "size too small", res: engine.Result{StatusCode: 200, ContentLength: 50}, want: false},
		{name: "size too large", res: engine.Result{StatusCode: 200, ContentLength: 600}, want: false},
		{name: "error result", res: engine.Result{Err: errors.New("boom")}, want: true},
		{name: "unknown size", res: engine.Result{StatusCode: 200, ContentLength: -1}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matcher.Matches(tt.res)
			if got != tt.want {
				t.Fatalf("got %v want %v", got, tt.want)
			}
		})
	}
}

func TestMatcherBaselineSimilarity(t *testing.T) {
	baseline := []byte("This is the default 404 page. Nothing to see here.")
	matcher := New(Options{
		SimilarityThreshold: 0.6,
		BaselineBody:        baseline,
	})

	similar := engine.Result{
		StatusCode:    404,
		ContentLength: 150,
		Body:          []byte("This is the default 404 page nothing to see here with maybe a link."),
	}

	similarity := jaccardSimilarity(matcher.baseline, buildShingles(similar.Body, matcher.shingleSize))
	if similarity < 0.6 {
		t.Fatalf("expected similarity >= 0.6, got %f", similarity)
	}

	if matcher.Matches(similar) {
		t.Fatalf("expected similar body to be filtered")
	}

	different := engine.Result{
		StatusCode:    404,
		ContentLength: 120,
		Body:          []byte("Welcome to the admin panel"),
	}

	if !matcher.Matches(different) {
		t.Fatalf("expected different body to pass")
	}

	emptyBody := engine.Result{StatusCode: 404}
	if !matcher.Matches(emptyBody) {
		t.Fatalf("expected empty body to pass when baseline filtering enabled")
	}
}

func TestMatcherEvaluateReportsSimilarity(t *testing.T) {
	baseline := []byte("baseline response body for comparison")
	matcher := New(Options{
		SimilarityThreshold: 0.5,
		BaselineBody:        baseline,
	})

	similar := engine.Result{Body: []byte("baseline response body for comparison and extras")}
	outcome := matcher.Evaluate(similar)
	if !outcome.HasSimilarity {
		t.Fatalf("expected similarity metadata")
	}
	if outcome.Similarity <= 0 {
		t.Fatalf("expected positive similarity, got %f", outcome.Similarity)
	}
}
