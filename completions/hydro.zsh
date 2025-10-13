#compdef hydro

_arguments \
  '--beginner[Enable beginner-friendly defaults]' \
  '--burp-export[Write matched requests and responses to a Burp-compatible XML file]:value:_guard "^-" "option argument"' \
  '--concurrency[Number of concurrent workers]:value:_guard "^-" "option argument"' \
  '--filter-size[Filter visible hits by response size range (min-max bytes)]:value:_guard "^-" "option argument"' \
  '--follow-redirects[Follow HTTP redirects (up to 5 hops)]' \
  '-h[Show usage information]' \
  '--help[Show usage information]' \
  '--match-status[Comma-separated list of HTTP status codes to include in hits]:value:_guard "^-" "option argument"' \
  '--method[HTTP method to use for requests (GET, HEAD, POST)]:value:_guard "^-" "option argument"' \
  '--no-baseline[Disable the automatic baseline request used for similarity filtering]' \
  '--output[Path to write output results]:value:_guard "^-" "option argument"' \
  '--output-format[Format for --output (jsonl)]:value:_guard "^-" "option argument"' \
  '--pre-hook[Shell command to run once before requests to fetch auth headers (stdout JSON)]:value:_guard "^-" "option argument"' \
  '--profile[Named execution profile to load]:value:_guard "^-" "option argument"' \
  '--resume[Path to a SQLite database for resuming and recording runs]:value:_guard "^-" "option argument"' \
  '--run-id[Override the deterministic run identifier used for persistence]:value:_guard "^-" "option argument"' \
  '--show-similarity[Include similarity scores in output (debug)]' \
  '--similarity-threshold[Hide hits whose bodies are this similar to the baseline (0-1)]:value:_guard "^-" "option argument"' \
  '--timeout[Request timeout duration]:value:_guard "^-" "option argument"' \
  '-u[Target URL or template (required)]:value:_guard "^-" "option argument"' \
  '-w[Path to the wordlist file (required)]:value:_guard "^-" "option argument"'

