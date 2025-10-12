package templater

import (
	"fmt"
	"strconv"
	"strings"
)

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

// ExpandPayload returns the list of payloads obtained by expanding ffuf-style
// brace expressions ("{a,b}") and numeric ranges ("[1-10]") found within the
// provided payload string. When no expandable expressions are found, the
// original payload is returned.
func (t *Templater) ExpandPayload(payload string) []string {
	if payload == "" {
		return []string{""}
	}

	results := []string{payload}

	for {
		expanded := make([]string, 0, len(results))
		changed := false

		for _, current := range results {
			variants, ok := expandOnce(current)
			if ok {
				expanded = append(expanded, variants...)
				changed = true
				continue
			}

			expanded = append(expanded, current)
		}

		results = expanded
		if !changed {
			break
		}
	}

	return results
}

func expandOnce(payload string) ([]string, bool) {
	for i := 0; i < len(payload); i++ {
		switch payload[i] {
		case '{':
			closing := strings.IndexByte(payload[i+1:], '}')
			if closing < 0 {
				continue
			}

			closing += i + 1
			options := strings.Split(payload[i+1:closing], ",")
			if len(options) == 0 {
				continue
			}

			prefix := payload[:i]
			suffix := payload[closing+1:]

			expanded := make([]string, 0, len(options))
			for _, opt := range options {
				expanded = append(expanded, prefix+strings.TrimSpace(opt)+suffix)
			}

			return expanded, true

		case '[':
			closing := strings.IndexByte(payload[i+1:], ']')
			if closing < 0 {
				continue
			}

			closing += i + 1
			contents := strings.TrimSpace(payload[i+1 : closing])
			parts := strings.SplitN(contents, "-", 2)
			if len(parts) != 2 {
				continue
			}

			startStr := strings.TrimSpace(parts[0])
			endStr := strings.TrimSpace(parts[1])

			if startStr == "" || endStr == "" {
				continue
			}

			start, err := strconv.Atoi(startStr)
			if err != nil {
				continue
			}

			end, err := strconv.Atoi(endStr)
			if err != nil {
				continue
			}

			prefix := payload[:i]
			suffix := payload[closing+1:]

			width := 0
			if len(startStr) == len(endStr) && (strings.HasPrefix(startStr, "0") || strings.HasPrefix(endStr, "0")) {
				width = len(startStr)
			}

			step := 1
			if start > end {
				step = -1
			}

			count := 1 + abs(start-end)
			expanded := make([]string, 0, count)
			for value := start; ; value += step {
				var formatted string
				if width > 0 {
					formatted = fmt.Sprintf("%0*d", width, value)
				} else {
					formatted = strconv.Itoa(value)
				}

				expanded = append(expanded, prefix+formatted+suffix)

				if value == end {
					break
				}
			}

			return expanded, true
		}
	}

	return nil, false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
