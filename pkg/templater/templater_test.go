package templater

import "testing"

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
