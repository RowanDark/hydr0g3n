# Quickstart Guide

Welcome to **hydro**, a fast content-discovery tool designed to help you enumerate HTTP paths with confidence. This tutorial walks through the basics of running your first scan, introduces the most common command-line switches, and shares safety tips so you can test responsibly.

## Prerequisites

1. Install Go 1.22 or later.
2. Clone this repository and build the CLI:
   ```bash
   git clone https://github.com/example/hydr0g3n.git
   cd hydr0g3n
   go build ./cmd/hydro
   ```
3. Ensure the binary is on your `PATH` (or reference it via `./hydro`).
4. Prepare a wordlist file containing the paths you want to probe. You can start with the bundled files under [`examples/`](../examples/).

## Step-by-step: your first run

1. **Select a target.** Decide which host or endpoint you have permission to test.
2. **Pick a wordlist.** For quick experiments, use [`examples/sample_small.txt`](../examples/sample_small.txt).
3. **Run hydro with beginner defaults:**
   ```bash
   ./hydro --beginner -u https://example.com -w examples/sample_small.txt
   ```
4. **Review the output.** Hits are printed as JSON lines by default. Each line includes the path, status code, and response size. When watching the run in your terminal, switch to a hierarchical tree with `--view tree` and control ANSI colors with `--color-mode` (`auto`, `always`, `never`) plus `--color-preset` (`default`, `protanopia`, `tritanopia`, `blue-light`).
5. **Adjust scope.** Swap in a larger list (for example [`examples/common.txt`](../examples/common.txt)) or point at templated URLs like `https://example.com/blog/FUZZ`.
6. **Iterate safely.** Tweak concurrency and timeouts gradually, watching for rate limits or defensive responses from the target.

## Example commands

The commands below progress from basic to advanced usage. Feel free to copy them into your own workflow and adapt paths or domains as needed.

1. **Beginner-friendly defaults:**
   ```bash
   ./hydro --beginner -u https://example.com -w examples/sample_small.txt
   ```
2. **Directory brute force with an explicit template:**
   ```bash
   ./hydro -u https://example.com/blog/FUZZ -w wordlists/directories.txt --method GET
   ```
3. **Only show specific HTTP statuses:**
   ```bash
   ./hydro -u https://api.example.com/v1/FUZZ -w examples/common.txt --match-status 200,204,403
   ```
4. **Filter by response body size and follow redirects:**
   ```bash
   ./hydro -u https://files.example.com/FUZZ -w examples/common.txt --filter-size 200-1024 --follow-redirects
   ```
5. **Persist state and resume later:**
   ```bash
   ./hydro -u https://shop.example.com/FUZZ -w examples/common.txt --resume runs/shop.sqlite --run-id nightly
   ```
6. **Leverage advanced output and hooks:**
   ```bash
   ./hydro -u https://admin.example.com/FUZZ -w examples/common.txt --output results.jsonl --output-format jsonl --pre-hook './scripts/auth.sh'
   ```
7. **Similarity-aware fuzzing with GET requests:**
   ```bash
   ./hydro -u https://intranet.example.com/FUZZ -w examples/common.txt --method GET --similarity-threshold 0.4 --show-similarity
   ```
8. **Tree view with a colorblind-friendly palette:**
   ```bash
   ./hydro -u https://portal.example.com/FUZZ -w examples/common.txt --view tree --color-mode always --color-preset protanopia
   ```

## Understanding `HEAD` vs `GET`

`hydro` defaults to the HTTP `HEAD` method. A `HEAD` request retrieves only response headers and the status code, skipping the body. This makes enumeration faster and lighter on the target. Switching to `GET` downloads the full body, which:

- Enables similarity filtering and richer analysis when content matters.
- Consumes more bandwidth and may trigger rate limits sooner.
- Increases the risk of affecting stateful endpoints if they rely on side effects.

Choose `HEAD` for broad reconnaissance, and switch to `GET` only when you need to inspect bodies or confirm interesting findings. The `--method` flag also accepts `POST` when you need to probe endpoints that require a request body.

## Safety checklist

- **Obtain authorization** before touching a host. Written permission or program scope documentation is essential.
- **Respect rate limits.** Start with low concurrency (`--concurrency 5`) and increase slowly.
- **Avoid destructive verbs.** Stick with safe methods (`HEAD`/`GET`) unless you understand the impact of `POST` requests.
- **Log responsibly.** Use `--output` to keep an audit trail, and protect any sensitive data you gather.
- **Stop on anomalies.** If you see unexpected status codes (500s) or the target slows down, pause and reassess.

## Troubleshooting

| Symptom | Possible cause | Suggested fix |
|---------|----------------|----------------|
| `hydro: a target URL must be provided with -u` | Required flag missing | Re-run the command with `-u https://...` |
| Requests time out immediately | Network filtering or very slow host | Increase `--timeout` or test connectivity with `curl` first |
| No hits even on known paths | Wordlist doesn’t match or baseline filtering hides responses | Try `--no-baseline`, lower `--similarity-threshold`, or confirm the URL template is correct |
| Rate limiting errors (429) | Too many concurrent requests | Reduce `--concurrency` and add delays via external tooling |
| `unsupported HTTP method` | Typo in `--method` flag | Use `HEAD`, `GET`, or `POST` only |

You’re now ready to dive deeper. Explore the `--profile` system in [`pkg/config`](../pkg/config) and automate complex workflows with the example playbook in [`examples/example-playbook.md`](../examples/example-playbook.md).
