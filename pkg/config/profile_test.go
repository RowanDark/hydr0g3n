package config

import (
	"testing"
	"time"
)

func TestLookupProfileBeginner(t *testing.T) {
	profile, ok := LookupProfile("beginner")
	if !ok {
		t.Fatal("expected beginner profile to be available")
	}

	if profile.Method != "HEAD" {
		t.Fatalf("expected method HEAD, got %q", profile.Method)
	}

	if profile.Concurrency != 10 {
		t.Fatalf("expected concurrency 10, got %d", profile.Concurrency)
	}

	if profile.Throttle != 100*time.Millisecond {
		t.Fatalf("expected throttle 100ms, got %v", profile.Throttle)
	}

	if profile.Recursive {
		t.Fatal("expected recursion disabled for beginner profile")
	}

	if profile.Timeout != 10*time.Second {
		t.Fatalf("expected timeout 10s, got %v", profile.Timeout)
	}

	expectedOutputs := []string{"pretty", "jsonl"}
	if len(profile.Outputs) != len(expectedOutputs) {
		t.Fatalf("expected %d outputs, got %d", len(expectedOutputs), len(profile.Outputs))
	}

	for i, v := range expectedOutputs {
		if profile.Outputs[i] != v {
			t.Fatalf("expected output[%d] to be %q, got %q", i, v, profile.Outputs[i])
		}
	}
}

func TestLookupProfileCaseInsensitive(t *testing.T) {
	_, ok := LookupProfile("Beginner")
	if !ok {
		t.Fatal("lookup should be case-insensitive")
	}
}

func TestLookupProfileUnknown(t *testing.T) {
	_, ok := LookupProfile("unknown")
	if ok {
		t.Fatal("expected lookup to fail for unknown profile")
	}
}
