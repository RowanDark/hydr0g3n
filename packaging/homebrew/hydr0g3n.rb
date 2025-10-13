# typed: false
# frozen_string_literal: true

# GoReleaser template for the hydr0g3n Homebrew formula. The macOS and Linux
# sections are rendered dynamically based on the packages produced during a
# release.
{{- define "macos_packages" }}
{{- range $element := .MacOSPackages }}
  {{- if eq $element.Arch "all" }}
  url "{{ $element.DownloadURL }}"
  {{- if .DownloadStrategy }}, using: {{ .DownloadStrategy }}{{- end }}
  {{- if .Headers }},
    headers: [{{ printf "\n" }}
      {{- join .Headers | indent 8 }}
    ]
  {{- end }}
  sha256 "{{ $element.SHA256 }}"

  def install
    {{- range $index, $element := .Install }}
    {{ . -}}
    {{- end }}
    bash_completion.install "completions/hydro.bash" => "hydro"
    zsh_completion.install "completions/hydro.zsh" => "_hydro"
    fish_completion.install "completions/hydro.fish"
    man1.install "man/hydro.1"
  end
  {{- else if $.HasOnlyAmd64MacOsPkg }}
  url "{{ $element.DownloadURL }}"
  {{- if .DownloadStrategy }}, using: {{ .DownloadStrategy }}{{- end }}
  {{- if .Headers }},
    headers: [{{ printf "\n" }}
      {{- join .Headers | indent 8 }}
    ]
  {{- end }}
  sha256 "{{ $element.SHA256 }}"

  def install
    {{- range $index, $element := .Install }}
    {{ . -}}
    {{- end }}
  end

  if Hardware::CPU.arm?
    def caveats
      <<~EOS
        The darwin_arm64 architecture is not supported for the {{ $.Name }}
        formula at this time. The darwin_amd64 binary may work in compatibility
        mode, but it might not be fully supported.
      EOS
    end
  end
  {{- else }}
  {{- if eq $element.Arch "amd64" }}
  if Hardware::CPU.intel?
  {{- end }}
  {{- if eq $element.Arch "arm64" }}
  if Hardware::CPU.arm?
  {{- end}}
    url "{{ $element.DownloadURL }}"
    {{- if .DownloadStrategy }}, using: {{ .DownloadStrategy }}{{- end }}
    {{- if .Headers }},
      headers: [{{ printf "\n" }}
        {{- join .Headers | indent 8 }}
      ]
    {{- end }}
    sha256 "{{ $element.SHA256 }}"

    def install
      {{- range $index, $element := .Install }}
      {{ . -}}
      {{- end }}
      bash_completion.install "completions/hydro.bash" => "hydro"
      zsh_completion.install "completions/hydro.zsh" => "_hydro"
      fish_completion.install "completions/hydro.fish"
      man1.install "man/hydro.1"
    end
  end
  {{- end }}
{{- end }}
{{- end }}

{{- define "linux_packages" }}
{{- range $element := .LinuxPackages }}
  {{- if eq $element.Arch "amd64" }}
  if Hardware::CPU.intel? && Hardware::CPU.is_64_bit?
  {{- else if eq $element.Arch "arm64" }}
  if Hardware::CPU.arm? && Hardware::CPU.is_64_bit?
  {{- else if eq $element.Arch "arm" }}
  if Hardware::CPU.arm? && !Hardware::CPU.is_64_bit?
  {{- end }}
    url "{{ $element.DownloadURL }}"
    {{- if .DownloadStrategy }}, using: {{ .DownloadStrategy }}{{- end }}
    {{- if .Headers }},
      headers: [{{ printf "\n" }}
        {{- join .Headers | indent 8 }}
      ]
    {{- end }}
    sha256 "{{ $element.SHA256 }}"
    def install
    {{- range $index, $element := .Install }}
      {{ . -}}
    {{- end }}
    end
  end
{{- end }}
{{- end }}

class Hydr0g3n < Formula
  desc "{{ .Desc }}"
  homepage "{{ .Homepage }}"
  version "{{ .Version }}"
  {{- if .License }}
  license "{{ .License }}"
  {{- end }}

  {{- if and (not .LinuxPackages) .MacOSPackages }}
  depends_on :macos
  {{- end }}
  {{- if and (not .MacOSPackages) .LinuxPackages }}
  depends_on :linux
  {{- end }}

  {{- if and .MacOSPackages .LinuxPackages }}
  on_macos do
  {{- template "macos_packages" . | indent 4 }}
  end

  on_linux do
  {{- template "linux_packages" . | indent 4 }}
  end
  {{- end }}

  {{- if and (.MacOSPackages) (not .LinuxPackages) }}
  {{- template "macos_packages" . }}
  {{- end }}

  {{- if and (not .MacOSPackages) (.LinuxPackages) }}
  {{- template "linux_packages" . }}
  {{- end }}

  {{- if .Tests }}
  test do
    {{- range $index, $element := .Tests }}
    {{ . -}}
    {{- end }}
  end
  {{- else }}
  test do
    system "#{bin}/hydro", "--version"
  end
  {{- end }}
end
