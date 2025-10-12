# Example Hydro Playbook

This sample playbook demonstrates how you can stitch together multiple `hydro` runs to investigate a new host safely.

## 1. Baseline reconnaissance

```bash
./hydro --beginner -u https://target.example -w examples/sample_small.txt --output runs/target-baseline.jsonl
```

*Purpose:* identify obvious entry points quickly while collecting JSONL logs for later review.

## 2. Directory expansion with tuned filters

```bash
./hydro -u https://target.example/FUZZ -w examples/common.txt \
  --match-status 200,204,403 \
  --filter-size 400-2048 \
  --follow-redirects \
  --concurrency 15
```

*Purpose:* focus on meaningful hits, ignore tiny placeholder pages, and respect redirect flows.

## 3. Authenticated probing via pre-hook

```bash
./hydro -u https://target.example/internal/FUZZ -w wordlists/advanced.txt \
  --pre-hook './scripts/fetch-auth-token.sh' \
  --method GET \
  --similarity-threshold 0.45 \
  --resume runs/internal.sqlite
```

*Purpose:* fetch fresh credentials before the scan, compare bodies for near-duplicates, and make the run resumable.

## 4. Reporting

Combine the JSONL output into a single report using your favorite tooling (jq, Python, etc.) and share only with authorized stakeholders.

> 🔐 **Reminder:** Always follow the target's security policy and document consent before running any scans.
