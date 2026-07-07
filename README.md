# gojobs-ca

Find **Go / Golang jobs in Canada** by querying company career boards directly —
not by scraping third-party job sites.

## The idea

You don't scrape thousands of company websites' raw HTML (fragile, breaks
constantly). Instead you hit the **ATS (Applicant Tracking System)** that each
company already uses. A handful of ATS platforms host most tech job boards, and
each exposes a public, structured JSON endpoint per company:

| ATS        | Endpoint used                                                        |
|------------|----------------------------------------------------------------------|
| Greenhouse | `https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true`|
| Lever      | `https://api.lever.co/v0/postings/{slug}?mode=json`                  |
| Ashby      | `https://api.ashbyhq.com/posting-api/job-board/{slug}`               |

So "scrape every company site" becomes "query the ATS for each company on my
list" — reliable, fast, and no HTML parsing. The tool fetches all companies
concurrently, keeps only postings that look like **Go roles** *and* are in
**Canada**, and stores them in `jobs.json`. A small web UI lets you browse them.

## Requirements

- Go 1.22+ (`go version`). No external dependencies — standard library only.

## Quick start

```bash
go build -o gojobs-ca .

./gojobs-ca scrape        # fetch + filter into jobs.json
./gojobs-ca serve         # open http://localhost:8080
```

Re-run `scrape` anytime; results are merged and de-duplicated by URL, so the
job list accumulates and stays current.

## Commands

```
gojobs-ca discover [flags]
  -companies string   companies list to update (default "companies.json")
  -threads   int      recent HN threads to scan (default 6)
  -concurrency int    parallel board checks     (default 6)
  -timeout   duration per-request timeout        (default 30s)

gojobs-ca scrape [flags]
  -companies string   companies list           (default "companies.json")
  -out       string   output file              (default "jobs.json")
  -remote             also keep remote roles that don't name a Canadian city
  -concurrency int    parallel fetches         (default 8)
  -timeout   duration per-request timeout       (default 20s)
  -max-age-days int   drop postings older than N days (default 60; 0 = no limit)
  -discover           refresh companies.json from HN first, then scrape
  -threads   int      with -discover: recent HN threads to scan (default 6)
  -ai                 judge relevance with the Claude API (needs ANTHROPIC_API_KEY)
  -ai-cli             judge relevance via the Claude Code CLI (uses your subscription)
  -model     string   Claude model for -ai/-ai-cli

gojobs-ca serve [flags]
  -jobs string        jobs file    (default "jobs.json")
  -addr string        listen addr  (default ":8080")
  -max-age-days int   hide postings older than N days (default 60; 0 = show all)
```

Tip: add `-remote` to catch "Remote (North America)" style postings that don't
explicitly say Canada. Without it, only postings naming a Canadian location
(or an explicitly remote-in-Canada posting) are kept.

### Freshness (`-max-age-days`)

By default only postings from the **last 60 days** (~2 months) are kept;
anything older is dropped before the Go/Canada check, and postings that age past
the window are also pruned from `jobs.json` on the next run — so the list never
accumulates stale roles. A posting whose ATS record has no date is treated as
too old and dropped. Change the window with `-max-age-days 30`, or disable the
limit entirely with `-max-age-days 0`.

`serve` applies the **same window on every request**, computed relative to the
moment the page is hit. So a posting silently disappears once it ages past the
limit even without a re-scrape, and any stale entry in `jobs.json` is never
shown. It takes the same `-max-age-days` flag (default 60).

## Building the company list automatically (`discover`)

Instead of hand-curating `companies.json`, let the tool find Go employers for
you:

```bash
gojobs-ca discover        # refresh companies.json from HN "Who is hiring?"
gojobs-ca scrape          # then scrape them
# or in one step:
gojobs-ca scrape -discover
```

How it works (`discover.go`), all from real, no-auth sources:

1. Pulls the last few monthly **"Ask HN: Who is hiring?"** threads via the
   public HN Algolia API.
2. Scans every comment that mentions Go/Golang for a Greenhouse, Lever, or
   Ashby board link, and extracts the `{ats, slug}`.
3. **Verifies** each candidate against its live ATS board and keeps only the
   ones that resolve with at least one open posting (dead slugs and empty
   boards are dropped).
4. Merges the new companies into `companies.json`, de-duplicated against what's
   already there — so re-running is safe and only ever adds.

Company names are derived from the slug as a best effort (e.g. `coram-ai` →
"Coram Ai") — rename them in `companies.json` if you like. Scan more history
with `-threads 12` for a bigger list.

## Adding companies manually

`companies.json` is just a list. Add any company on Greenhouse, Lever, or Ashby:

```json
{ "name": "Acme", "ats": "greenhouse", "slug": "acme" }
```

**Finding the slug** (the important part): open the company's careers page and
look at the URL or the network tab.

- Greenhouse boards look like `boards.greenhouse.io/acme` or
  `job-boards.greenhouse.io/acme` → slug is `acme`.
- Lever boards look like `jobs.lever.co/acme` → slug is `acme`.
- Ashby boards look like `jobs.ashbyhq.com/acme` → slug is `acme`.

If a slug is wrong the tool logs a `404 — slug not found` and skips it, so a bad
entry never breaks a run. **The shipped `companies.json` is a starter list — the
slugs are best guesses and you should verify each one.** The value of the tool is
your own curated company list; the more Go-heavy Canadian companies you add, the
better it gets.

## How matching works (`filter.go`)

- **Go role:** `golang` anywhere in the title or description, *or* a standalone
  `Go` in the title next to a role word (Engineer / Developer / Backend / …).
  A bare "go" in prose ("ready to go", "go-to-market") is deliberately ignored.
- **Canada:** the location or description names Canada, a province, or a major
  Canadian city. Remote handling is controlled by the `-remote` flag.

Both rules are simple regexes — tune them to taste. Run `go test ./...` after
changes; `filter_test.go` covers the tricky cases.

## Relevance via Claude (`-ai`)

The keyword filter is fast and free but blunt: it can't tell a real Go backend
role from a posting that merely says "go-to-market", and it treats a job as
"in Canada" if the description mentions Canada anywhere — even when the role's
location is elsewhere.

Pass `-ai` to have **Claude** judge each posting instead. The tool fetches all
raw postings as usual, then sends each one's title / location / snippet to the
Anthropic Messages API (in batches) and asks whether it's a genuine Go
software-engineering role located in Canada. Only postings Claude approves are
written to `jobs.json`.

```bash
export ANTHROPIC_API_KEY=sk-ant-...
./gojobs-ca scrape -ai                       # classify with the default model
./gojobs-ca scrape -ai -model claude-haiku-4-5   # cheaper/faster classification
```

- Needs `ANTHROPIC_API_KEY` in the environment; the run aborts immediately if
  it's unset (nothing is fetched).
- Default model is `claude-opus-4-8` for best judgment; `claude-haiku-4-5` is a
  much cheaper option for a bulk pass over hundreds of postings.
- `-remote` still applies: with `-ai -remote`, remote roles Claude flags as
  Go-relevant are kept even without a named Canadian city.
- No SDK dependency — the call is a plain `net/http` POST (see `ai.go`), so the
  standard-library-only promise holds. Batches that error out are logged and
  skipped rather than failing the whole run.

### No API key? Use your Claude Code subscription (`-ai-cli`)

A Claude Pro/Max subscription doesn't cover direct API calls, but it does cover
the Claude Code CLI. `-ai-cli` runs the exact same classification through
`claude -p` (headless mode) instead of the HTTP API:

```bash
./gojobs-ca scrape -ai-cli                # billed to your subscription
./gojobs-ca scrape -ai-cli -model haiku   # optional model override
```

- Needs the `claude` CLI installed and logged in. Install it with
  `npm install -g @anthropic-ai/claude-code`, or on Windows PowerShell:
  `irm https://claude.ai/install.ps1 | iex` — then run `claude` once and log in.
- Slower than `-ai` (CLI startup per batch) and subject to your plan's usage
  limits, but costs nothing beyond the subscription.
- Without `-model` it uses whatever model your CLI session defaults to.

## Ideas to extend

- Add more ATS providers (Workday, SmartRecruiters, Recruitee, Workable).
- Add a cron/systemd timer to `scrape` daily and diff for *new* postings.
- Email/Slack a digest of postings added since the last run.
- Add seniority or salary parsing to the filter.

## A note on etiquette

These are public endpoints, but be polite: the default concurrency (8) and a
once-a-day scrape are plenty. Don't hammer them.
