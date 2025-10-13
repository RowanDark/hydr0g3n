package main

import (
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"
)

type completionFlag struct {
	Name    string
	Usage   string
	IsBool  bool
	IsShort bool
}

func collectCompletionFlags() []completionFlag {
	var flags []completionFlag
	seen := make(map[string]struct{})
	flag.CommandLine.VisitAll(func(f *flag.Flag) {
		if f.Name == "completion-script" {
			return
		}

		usage := strings.TrimSpace(f.Usage)

		isBool := false
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok {
			isBool = bf.IsBoolFlag()
		}

		flags = append(flags, completionFlag{
			Name:    f.Name,
			Usage:   usage,
			IsBool:  isBool,
			IsShort: len(f.Name) == 1,
		})
		seen[f.Name] = struct{}{}
	})

	if _, ok := seen["help"]; !ok {
		flags = append(flags, completionFlag{
			Name:   "help",
			Usage:  "Show usage information",
			IsBool: true,
		})
	}
	if _, ok := seen["h"]; !ok {
		flags = append(flags, completionFlag{
			Name:    "h",
			Usage:   "Show usage information",
			IsBool:  true,
			IsShort: true,
		})
	}

	sort.Slice(flags, func(i, j int) bool {
		return flags[i].Name < flags[j].Name
	})

	return flags
}

func outputCompletionScript(w io.Writer, shell string) error {
	flags := collectCompletionFlags()

	var (
		script string
		err    error
	)

	switch shell {
	case "bash":
		script, err = renderBashCompletion(flags)
	case "zsh":
		script, err = renderZshCompletion(flags)
	case "fish":
		script, err = renderFishCompletion(flags)
	default:
		return fmt.Errorf("unsupported shell %q: choose from bash, zsh, fish", shell)
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, script)
	return err
}

func renderBashCompletion(flags []completionFlag) (string, error) {
	var opts []string
	for _, f := range flags {
		prefix := "--"
		if f.IsShort {
			prefix = "-"
		}
		opts = append(opts, prefix+f.Name)
	}

	sort.Strings(opts)

	data := struct {
		Options string
	}{
		Options: strings.Join(opts, " "),
	}

	var buf strings.Builder
	if err := bashTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderZshCompletion(flags []completionFlag) (string, error) {
	type entry struct {
		Option string
		Usage  string
		HasArg bool
	}

	var entries []entry
	for _, f := range flags {
		option := "--" + f.Name
		if f.IsShort {
			option = "-" + f.Name
		}
		entries = append(entries, entry{
			Option: option,
			Usage:  escapeForZsh(f.Usage),
			HasArg: !f.IsBool,
		})
	}

	data := struct {
		Entries []entry
	}{
		Entries: entries,
	}

	var buf strings.Builder
	if err := zshTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func renderFishCompletion(flags []completionFlag) (string, error) {
	type entry struct {
		Short  string
		Long   string
		Usage  string
		HasArg bool
	}

	var entries []entry
	for _, f := range flags {
		e := entry{Usage: escapeForFish(f.Usage), HasArg: !f.IsBool}
		if f.IsShort {
			e.Short = f.Name
		} else {
			e.Long = f.Name
		}
		entries = append(entries, e)
	}

	data := struct {
		Entries []entry
	}{
		Entries: entries,
	}

	var buf strings.Builder
	if err := fishTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

var bashTemplate = template.Must(template.New("bash").Parse(`# bash completion for hydro
_hydro_completions()
{
    local cur opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    opts="{{ .Options }}"

    if [[ ${cur} == -* ]]; then
        COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
        return 0
    fi
}

complete -F _hydro_completions hydro`))

var zshTemplate = template.Must(template.New("zsh").Funcs(template.FuncMap{
	"plus": func(a, b int) int { return a + b },
}).Parse(`#compdef hydro

_arguments \
{{- range $index, $flag := .Entries }}
  '{{ $flag.Option }}{{ if $flag.Usage }}[{{ $flag.Usage }}]{{ end }}{{ if $flag.HasArg }}:value:_guard "^-" "option argument"{{ end }}'{{- if lt (plus $index 1) (len $.Entries) }} \
{{- end }}
{{- end }}
`))

var fishTemplate = template.Must(template.New("fish").Parse(`# fish completion for hydro
{{- range .Entries }}
complete -c hydro{{ if .Short }} -s {{ .Short }}{{ end }}{{ if .Long }} -l {{ .Long }}{{ end }}{{ if .HasArg }} -r{{ end }}{{ if .Usage }} -d '{{ .Usage }}'{{ end }}
{{- end }}`))

func escapeForZsh(input string) string {
	if input == "" {
		return ""
	}

	escaped := strings.ReplaceAll(input, "'", "'\\''")
	escaped = strings.ReplaceAll(escaped, ":", `\:`)
	escaped = strings.ReplaceAll(escaped, "[", `\[`)
	escaped = strings.ReplaceAll(escaped, "]", `\]`)
	return escaped
}

func escapeForFish(input string) string {
	return strings.ReplaceAll(input, "'", "\\'")
}
