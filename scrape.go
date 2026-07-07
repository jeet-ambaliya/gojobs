package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

func runScrape(args []string) {
	fs := flag.NewFlagSet("scrape", flag.ExitOnError)
	companiesPath := fs.String("companies", "companies.json", "path to the companies list")
	outPath := fs.String("out", "jobs.json", "path to write matched jobs")
	includeRemote := fs.Bool("remote", false, "also keep remote roles that don't name a Canadian location")
	concurrency := fs.Int("concurrency", 8, "how many companies to fetch in parallel")
	timeout := fs.Duration("timeout", 20*time.Second, "per-request timeout")
	maxAgeDays := fs.Int("max-age-days", 60, "drop postings older than this many days (0 = no limit)")
	useAI := fs.Bool("ai", false, "judge relevance with the Claude API instead of the keyword filter (needs ANTHROPIC_API_KEY)")
	useCLI := fs.Bool("ai-cli", false, "judge relevance via the Claude Code CLI (claude -p) — covered by a Claude Code subscription")
	model := fs.String("model", "", "Claude model for -ai/-ai-cli (default: claude-opus-4-8 for -ai; your CLI default for -ai-cli)")
	discover := fs.Bool("discover", false, "first refresh companies.json from HN 'Who is hiring?' threads, then scrape")
	threads := fs.Int("threads", 6, "with -discover: how many recent 'Who is hiring?' threads to scan")
	_ = fs.Parse(args)

	// Fail fast on a bad AI config before we fetch anything.
	var classifier batchClassifier
	switch {
	case *useAI && *useCLI:
		log.Fatalf("pick one of -ai or -ai-cli, not both")
	case *useAI:
		ai, err := newAIClient(*model, 60*time.Second)
		if err != nil {
			log.Fatalf("ai: %v", err)
		}
		classifier = ai
	case *useCLI:
		cli, err := newCLIClient(*model)
		if err != nil {
			log.Fatalf("ai-cli: %v", err)
		}
		classifier = cli
	}

	client := &http.Client{Timeout: *timeout}

	// Optionally refresh the company list from HN before scraping.
	if *discover {
		if err := refreshCompanies(client, *companiesPath, *threads, *concurrency); err != nil {
			log.Fatalf("discover: %v", err)
		}
	}

	companies, err := loadCompanies(*companiesPath)
	if err != nil {
		log.Fatalf("load companies: %v", err)
	}
	if classifier != nil {
		log.Printf("Scraping %d companies (concurrency %d), classifying with %s…\n", len(companies), *concurrency, classifier.name())
	} else {
		log.Printf("Scraping %d companies (concurrency %d)…\n", len(companies), *concurrency)
	}

	// Postings older than this are dropped before any relevance check.
	var ageCutoff time.Time
	if *maxAgeDays > 0 {
		ageCutoff = time.Now().UTC().AddDate(0, 0, -*maxAgeDays)
	}

	sem := make(chan struct{}, *concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var matched []Job  // regex-mode: postings kept as we fetch
	var fetched []Job  // ai-mode: every posting, classified after all fetches
	var okCount, failCount, staleDropped int

	for _, c := range companies {
		wg.Add(1)
		sem <- struct{}{}
		go func(c Company) {
			defer wg.Done()
			defer func() { <-sem }()

			jobs, err := fetchCompany(client, c)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failCount++
				log.Printf("  ✗ %-24s %v", c.Name, err)
				return
			}
			okCount++

			// Hard age limit: drop anything posted before the cutoff (an
			// unknown/zero date sorts as year 1, so it's dropped too).
			if *maxAgeDays > 0 {
				recent := jobs[:0]
				for _, j := range jobs {
					if !j.PostedAt.Before(ageCutoff) {
						recent = append(recent, j)
					}
				}
				staleDropped += len(jobs) - len(recent)
				jobs = recent
			}

			// Parse the experience requirement now, while the full description
			// is still available (it's stripped before persisting).
			for i := range jobs {
				jobs[i].MinYears = parseYears(jobs[i].Title, jobs[i].Description)
			}

			if classifier != nil {
				// Defer relevance to Claude; just collect everything.
				fetched = append(fetched, jobs...)
				log.Printf("  ✓ %-24s %3d recent roles fetched", c.Name, len(jobs))
				return
			}

			kept := 0
			for _, j := range jobs {
				if !matchesGo(j.Title, j.Description) {
					continue
				}
				canada, remote := matchesCanada(j.Location, j.Description, j.Remote)
				if !(canada || (*includeRemote && remote)) {
					continue
				}
				j.Remote = remote
				j.Description = "" // don't persist full text
				matched = append(matched, j)
				kept++
			}
			log.Printf("  ✓ %-24s %3d roles → %d Go/CA match", c.Name, len(jobs), kept)
		}(c)
	}
	wg.Wait()

	if classifier != nil {
		log.Printf("Classifying %d postings with %s…", len(fetched), classifier.name())
		matched = classifyJobs(classifier, fetched, *includeRemote, log.Printf)
		log.Printf("  %d of %d judged relevant", len(matched), len(fetched))
	}

	merged := mergeJobs(loadJobsQuiet(*outPath), matched)
	// Enforce the age limit on the merged set too, so postings carried over
	// from earlier runs age out instead of lingering in jobs.json forever.
	if *maxAgeDays > 0 {
		fresh := merged[:0]
		for _, j := range merged {
			if !j.PostedAt.Before(ageCutoff) {
				fresh = append(fresh, j)
			}
		}
		merged = fresh
	}
	if err := saveJobs(*outPath, merged); err != nil {
		log.Fatalf("save: %v", err)
	}

	staleNote := ""
	if *maxAgeDays > 0 {
		staleNote = fmt.Sprintf(" (dropped %d postings older than %d days)", staleDropped, *maxAgeDays)
	}
	log.Printf("\nDone: %d ok, %d failed. %d total matching jobs in %s%s (run: gojobs-ca serve)",
		okCount, failCount, len(merged), *outPath, staleNote)
}
