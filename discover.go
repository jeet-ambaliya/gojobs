package main

import (
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// runDiscover is the `gojobs-ca discover` subcommand: refresh companies.json
// from HN without scraping.
func runDiscover(args []string) {
	fs := flag.NewFlagSet("discover", flag.ExitOnError)
	companiesPath := fs.String("companies", "companies.json", "path to the companies list to update")
	threads := fs.Int("threads", 6, "how many recent 'Who is hiring?' threads to scan")
	concurrency := fs.Int("concurrency", 6, "parallel board-verification fetches")
	timeout := fs.Duration("timeout", 30*time.Second, "per-request timeout")
	_ = fs.Parse(args)

	client := &http.Client{Timeout: *timeout}
	if err := refreshCompanies(client, *companiesPath, *threads, *concurrency); err != nil {
		log.Fatalf("discover: %v", err)
	}
}

// refreshCompanies discovers new Go-hiring companies and merges them into the
// list at path, writing it back. Shared by `discover` and `scrape -discover`.
func refreshCompanies(client *http.Client, path string, threads, concurrency int) error {
	existing := loadCompaniesQuiet(path) // missing file is fine — we'll create it
	added, err := discoverCompanies(client, discoverConfig{
		Threads:     threads,
		Concurrency: concurrency,
		Verify:      true,
	}, existing, log.Printf)
	if err != nil {
		return err
	}
	if len(added) == 0 {
		log.Printf("No new companies to add — %s already covers everything found (%d companies).", path, len(existing))
		return nil
	}
	merged := append(existing, added...)
	if err := saveCompanies(path, merged); err != nil {
		return err
	}
	log.Printf("Added %d new companies to %s (%d total):", len(added), path, len(merged))
	for _, c := range added {
		log.Printf("  + %-20s %-11s %s", c.Name, c.ATS, c.Slug)
	}
	return nil
}

// Discovery finds companies that post Go roles by mining the monthly
// "Ask HN: Who is hiring?" threads — a large, public, no-auth source where
// companies link their own ATS boards directly. We pull each thread's comment
// tree from the HN Algolia API, extract Greenhouse/Lever/Ashby slugs from
// comments that mention Go, then (optionally) verify each candidate resolves to
// a live board before adding it to the company list.

// atsURLRe pulls the ATS host + slug out of a careers link in a comment.
var atsURLRe = regexp.MustCompile(`https?://(?:www\.)?(boards\.greenhouse\.io|job-boards\.greenhouse\.io|jobs\.lever\.co|jobs\.ashbyhq\.com)/([A-Za-z0-9][A-Za-z0-9_.-]*)`)

// goMentionRe matches a comment that plausibly names Go: the word "golang"
// (any case) or a standalone capitalized "Go". Lowercase "go" is left out on
// purpose — it's noise ("go to", "good to go").
var goMentionRe = regexp.MustCompile(`(?i)\bgolang\b|\bGo\b`)

var hostToATS = map[string]string{
	"boards.greenhouse.io":     "greenhouse",
	"job-boards.greenhouse.io": "greenhouse",
	"jobs.lever.co":            "lever",
	"jobs.ashbyhq.com":         "ashby",
}

// hnItem is one node of the Algolia item tree (a story or a comment).
type hnItem struct {
	Text     string   `json:"text"`
	Children []hnItem `json:"children"`
}

// hnSearch is the shape of the Algolia story-search response.
type hnSearch struct {
	Hits []struct {
		ObjectID string `json:"objectID"`
		Title    string `json:"title"`
	} `json:"hits"`
}

// discoverConfig controls a discovery run.
type discoverConfig struct {
	Threads     int  // how many recent "Who is hiring?" threads to scan
	Concurrency int  // parallel verification fetches
	Verify      bool // only keep candidates whose board is live with ≥1 posting
}

// discoverCompanies mines HN for Go-hiring companies and returns the ones not
// already in `existing`. When cfg.Verify is set, each candidate is confirmed
// against its live ATS board first.
func discoverCompanies(client *http.Client, cfg discoverConfig, existing []Company, logf func(string, ...any)) ([]Company, error) {
	threadIDs, err := recentHiringThreads(client, cfg.Threads)
	if err != nil {
		return nil, err
	}
	if len(threadIDs) == 0 {
		return nil, fmt.Errorf("no 'Who is hiring?' threads found")
	}
	logf("Scanning %d 'Who is hiring?' thread(s) for Go roles on Greenhouse/Lever/Ashby…", len(threadIDs))

	// Collect candidates as ats/slug, counting mentions across threads.
	type key struct{ ats, slug string }
	mentions := map[key]int{}
	for _, id := range threadIDs {
		var root hnItem
		if err := httpGetJSON(client, "https://hn.algolia.com/api/v1/items/"+id, &root); err != nil {
			logf("  ✗ thread %s: %v", id, err)
			continue
		}
		scanComment(root, func(ats, slug string) { mentions[key{ats, slug}]++ })
	}

	// Drop anything already on the list (dedupe by ats+slug, case-insensitive).
	have := map[string]bool{}
	for _, c := range existing {
		have[strings.ToLower(c.ATS)+"/"+strings.ToLower(c.Slug)] = true
	}
	var candidates []Company
	for k := range mentions {
		if have[k.ats+"/"+k.slug] {
			continue
		}
		candidates = append(candidates, Company{Name: prettyName(k.slug), ATS: k.ats, Slug: k.slug})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Slug < candidates[j].Slug })
	logf("Found %d new candidate companies (mentioning Go with a public board).", len(candidates))

	if !cfg.Verify {
		return candidates, nil
	}
	return verifyCandidates(client, candidates, cfg.Concurrency, logf), nil
}

// recentHiringThreads returns the IDs of the most recent "Who is hiring?"
// stories posted by the whoishiring account, newest first.
func recentHiringThreads(client *http.Client, n int) ([]string, error) {
	if n < 1 {
		n = 1
	}
	// Over-fetch: the author also posts "freelancer" and "wants to be hired" threads.
	url := fmt.Sprintf("https://hn.algolia.com/api/v1/search_by_date?tags=story,author_whoishiring&query=hiring&hitsPerPage=%d", n*3)
	var res hnSearch
	if err := httpGetJSON(client, url, &res); err != nil {
		return nil, err
	}
	var ids []string
	for _, h := range res.Hits {
		t := strings.ToLower(h.Title)
		if strings.Contains(t, "who is hiring") && !strings.Contains(t, "freelanc") {
			ids = append(ids, h.ObjectID)
			if len(ids) == n {
				break
			}
		}
	}
	return ids, nil
}

// scanComment walks a comment subtree and reports each ATS slug found in a
// comment that also mentions Go.
func scanComment(node hnItem, emit func(ats, slug string)) {
	for _, child := range node.Children {
		// HN encodes comment HTML — unescape so &#x2F; becomes / before matching.
		text := html.UnescapeString(child.Text)
		if goMentionRe.MatchString(text) {
			for _, m := range atsURLRe.FindAllStringSubmatch(text, -1) {
				slug := strings.ToLower(strings.Trim(m[2], "/."))
				if ats, ok := hostToATS[m[1]]; ok && slug != "" {
					emit(ats, slug)
				}
			}
		}
		scanComment(child, emit)
	}
}

// verifyCandidates keeps only the companies whose board is live and non-empty,
// fetching them in parallel. A dead slug or a board with zero postings is dropped.
func verifyCandidates(client *http.Client, candidates []Company, concurrency int, logf func(string, ...any)) []Company {
	if concurrency < 1 {
		concurrency = 6
	}
	logf("Verifying %d candidate board(s)…", len(candidates))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var kept []Company

	for _, c := range candidates {
		wg.Add(1)
		sem <- struct{}{}
		go func(c Company) {
			defer wg.Done()
			defer func() { <-sem }()

			jobs, err := fetchCompany(client, c)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				logf("  ✗ %-14s %-11s %s", c.Slug, c.ATS, err)
				return
			}
			if len(jobs) == 0 {
				logf("  – %-14s %-11s board empty, skipping", c.Slug, c.ATS)
				return
			}
			kept = append(kept, c)
			logf("  ✓ %-14s %-11s %d postings", c.Slug, c.ATS, len(jobs))
		}(c)
	}
	wg.Wait()

	sort.Slice(kept, func(i, j int) bool { return kept[i].Name < kept[j].Name })
	return kept
}

// prettyName turns an ATS slug into a best-effort display name. It's only a
// starting point — the user can rename entries in companies.json.
func prettyName(slug string) string {
	words := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' || r == '.' })
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
