// Command gojobs-ca finds Go/Golang jobs in Canada by querying the public
// job-board APIs of the ATS platforms companies use (Greenhouse, Lever,
// Ashby) — instead of scraping third-party job sites.
//
//	gojobs-ca scrape   # fetch + filter into jobs.json
//	gojobs-ca serve    # browse jobs.json in the browser
package main

import (
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "discover":
		runDiscover(os.Args[2:])
	case "scrape":
		runScrape(os.Args[2:])
	case "serve":
		runServe(os.Args[2:])
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `gojobs-ca — Go jobs in Canada, straight from company ATS boards

Usage:
  gojobs-ca discover [flags]  Refresh companies.json from HN "Who is hiring?"
  gojobs-ca scrape  [flags]   Fetch + filter roles into jobs.json
  gojobs-ca serve   [flags]   Serve a web UI to browse jobs.json

Common flags:
  discover -companies companies.json -threads 6
  scrape   -companies companies.json -out jobs.json -remote -concurrency 8
  scrape   -discover        # refresh the company list, then scrape
  serve    -jobs jobs.json -addr :8080

Typical run:
  gojobs-ca scrape -discover   # find Go employers + scrape in one step, then...
  gojobs-ca serve              # open http://localhost:8080
`)
}
