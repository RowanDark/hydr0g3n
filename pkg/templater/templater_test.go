package templater

import (
	"reflect"
	"testing"
)

func TestExpandDefaultPlaceholder(t *testing.T) {
	tpl := New()

	got := tpl.Expand("https://target/FUZZ", "admin")
	want := "https://target/admin"
	if got != want {
		t.Fatalf("Expand returned %q, want %q", got, want)
	}
}

func TestExpandMultipleOccurrences(t *testing.T) {
	tpl := New()

	got := tpl.Expand("FUZZ/and/FUZZ", "value")
	want := "value/and/value"
	if got != want {
		t.Fatalf("Expand returned %q, want %q", got, want)
	}
}

func TestExpandCurlyPlaceholder(t *testing.T) {
	tpl := New()

	got := tpl.Expand("https://target/{{FUZZ}}", "admin")
	want := "https://target/admin"
	if got != want {
		t.Fatalf("Expand returned %q, want %q", got, want)
	}
}

func TestExpandWithPrintfStyle(t *testing.T) {
	tpl := New()

	got := tpl.Expand("https://target/%s/profile", "admin")
	want := "https://target/admin/profile"
	if got != want {
		t.Fatalf("Expand returned %q, want %q", got, want)
	}
}

func TestExpandAppendToPath(t *testing.T) {
	tpl := New()

	tests := map[string]string{
		"https://target":   "https://target/admin",
		"https://target/":  "https://target/admin",
		"https://target//": "https://target//admin",
		"":                 "/admin",
	}

	for template, want := range tests {
		got := tpl.Expand(template, "admin")
		if got != want {
			t.Fatalf("Expand(%q) returned %q, want %q", template, got, want)
		}
	}
}

func TestExpandBody(t *testing.T) {
	tpl := New()

	got := tpl.Expand("username=FUZZ&password=FUZZ", "admin")
	want := "username=admin&password=admin"
	if got != want {
		t.Fatalf("Expand returned %q, want %q", got, want)
	}
}

func TestExpandMixedPlaceholders(t *testing.T) {
	tpl := New()

	got := tpl.Expand("{{FUZZ}}/FUZZ/%s", "payload")
	want := "payload/payload/payload"
	if got != want {
		t.Fatalf("Expand returned %q, want %q", got, want)
	}
}

func TestExpandPayloadBraces(t *testing.T) {
	tpl := New()

	got := tpl.ExpandPayload("FUZZ{.php,.html}")
	want := []string{"FUZZ.php", "FUZZ.html"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandPayload returned %v, want %v", got, want)
	}
}

func TestExpandPayloadRange(t *testing.T) {
	tpl := New()

	got := tpl.ExpandPayload("file[1-3]")
	want := []string{"file1", "file2", "file3"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandPayload returned %v, want %v", got, want)
	}
}

func TestExpandPayloadCombination(t *testing.T) {
	tpl := New()

	got := tpl.ExpandPayload("admin{,.php}[1-2]")
	want := []string{"admin1", "admin2", "admin.php1", "admin.php2"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandPayload returned %v, want %v", got, want)
	}
}

func TestExpandUsesDefaultPlaceholderWhenEmpty(t *testing.T) {
	tpl := NewWithPlaceholder("")

	got := tpl.Expand("https://target/FUZZ", "value")
	want := "https://target/value"

	if got != want {
		t.Fatalf("Expand returned %q, want %q", got, want)
	}
}
