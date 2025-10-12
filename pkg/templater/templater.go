package templater

import "strings"

// DefaultPlaceholder is the token substituted when expanding templates.
const DefaultPlaceholder = "FUZZ"

// Templater performs placeholder substitution on URL and body templates.
type Templater struct {
	placeholder string
}

// New creates a Templater configured with the DefaultPlaceholder token.
func New() *Templater {
	return &Templater{placeholder: DefaultPlaceholder}
}

// NewWithPlaceholder creates a Templater with a custom placeholder token. When
// placeholder is empty, DefaultPlaceholder is used.
func NewWithPlaceholder(placeholder string) *Templater {
	if placeholder == "" {
		placeholder = DefaultPlaceholder
	}

	return &Templater{placeholder: placeholder}
}

// Expand replaces placeholder occurrences within template using the provided
// payload and returns the expanded value. When the template does not contain a
// placeholder token, the payload is appended to the path.
func (t *Templater) Expand(template, payload string) string {
	if t == nil {
		return template
	}

	placeholder := t.placeholder
	if placeholder == "" {
		placeholder = DefaultPlaceholder
	}

	doublePlaceholder := "{{" + placeholder + "}}"
	expanded := template

	hasDouble := strings.Contains(template, doublePlaceholder)
	if hasDouble {
		expanded = strings.ReplaceAll(expanded, doublePlaceholder, payload)
	}

	templateWithoutDouble := template
	if hasDouble {
		templateWithoutDouble = strings.ReplaceAll(templateWithoutDouble, doublePlaceholder, "")
	}

	hasPlain := strings.Contains(templateWithoutDouble, placeholder)
	if hasPlain {
		expanded = strings.ReplaceAll(expanded, placeholder, payload)
	}

	hasFormat := strings.Contains(template, "%s")
	if hasFormat {
		expanded = strings.ReplaceAll(expanded, "%s", payload)
	}

	if hasDouble || hasPlain || hasFormat {
		return expanded
	}

	if strings.HasSuffix(template, "/") {
		return template + payload
	}
	return template + "/" + payload
}
