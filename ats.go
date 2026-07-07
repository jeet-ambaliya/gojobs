package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

// fetchCompany dispatches to the right ATS fetcher based on c.ATS.
func fetchCompany(client *http.Client, c Company) ([]Job, error) {
	switch strings.ToLower(c.ATS) {
	case "greenhouse":
		return fetchGreenhouse(client, c)
	case "lever":
		return fetchLever(client, c)
	case "ashby":
		return fetchAshby(client, c)
	default:
		return nil, fmt.Errorf("unknown ATS %q (use greenhouse, lever, or ashby)", c.ATS)
	}
}

// httpGetJSON performs a GET and decodes the JSON body into v.
func httpGetJSON(client *http.Client, url string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "gojobs-ca/1.0 (+personal job-search tool)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(resp.Body).Decode(v)
	case http.StatusNotFound:
		return fmt.Errorf("404 — slug not found, verify it on the careers page")
	default:
		return fmt.Errorf("http %d", resp.StatusCode)
	}
}

// preview trims a description to a short single-line snippet for the UI.
func preview(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 220 {
		s = s[:220] + "…"
	}
	return s
}

// ---- Greenhouse ---------------------------------------------------------
// Public board API: https://boards-api.greenhouse.io/v1/boards/{slug}/jobs?content=true

func fetchGreenhouse(client *http.Client, c Company) ([]Job, error) {
	url := fmt.Sprintf("https://boards-api.greenhouse.io/v1/boards/%s/jobs?content=true", c.Slug)
	var body struct {
		Jobs []struct {
			Title       string `json:"title"`
			AbsoluteURL string `json:"absolute_url"`
			UpdatedAt   string `json:"updated_at"`
			Content     string `json:"content"`
			Location    struct {
				Name string `json:"name"`
			} `json:"location"`
		} `json:"jobs"`
	}
	if err := httpGetJSON(client, url, &body); err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(body.Jobs))
	for _, j := range body.Jobs {
		// Greenhouse returns HTML-escaped content; unescape for matching.
		desc := stripTags(html.UnescapeString(j.Content))
		posted, _ := time.Parse(time.RFC3339, j.UpdatedAt)
		jobs = append(jobs, Job{
			ID:          j.AbsoluteURL,
			Company:     c.Name,
			Title:       j.Title,
			Location:    j.Location.Name,
			URL:         j.AbsoluteURL,
			Source:      "greenhouse",
			Snippet:     preview(desc),
			PostedAt:    posted,
			FetchedAt:   time.Now().UTC(),
			Description: desc,
		})
	}
	return jobs, nil
}

// ---- Lever --------------------------------------------------------------
// Public postings API: https://api.lever.co/v0/postings/{slug}?mode=json

func fetchLever(client *http.Client, c Company) ([]Job, error) {
	url := fmt.Sprintf("https://api.lever.co/v0/postings/%s?mode=json", c.Slug)
	var postings []struct {
		Text             string `json:"text"`
		HostedURL        string `json:"hostedUrl"`
		CreatedAt        int64  `json:"createdAt"` // epoch millis
		DescriptionPlain string `json:"descriptionPlain"`
		Categories       struct {
			Location   string `json:"location"`
			Team       string `json:"team"`
			Commitment string `json:"commitment"`
		} `json:"categories"`
		WorkplaceType string `json:"workplaceType"`
	}
	if err := httpGetJSON(client, url, &postings); err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		var posted time.Time
		if p.CreatedAt > 0 {
			posted = time.UnixMilli(p.CreatedAt).UTC()
		}
		jobs = append(jobs, Job{
			ID:          p.HostedURL,
			Company:     c.Name,
			Title:       p.Text,
			Location:    p.Categories.Location,
			URL:         p.HostedURL,
			Source:      "lever",
			Remote:      strings.EqualFold(p.WorkplaceType, "remote"),
			Snippet:     preview(p.DescriptionPlain),
			PostedAt:    posted,
			FetchedAt:   time.Now().UTC(),
			Description: p.DescriptionPlain,
		})
	}
	return jobs, nil
}

// ---- Ashby --------------------------------------------------------------
// Public job board API: https://api.ashbyhq.com/posting-api/job-board/{slug}

func fetchAshby(client *http.Client, c Company) ([]Job, error) {
	url := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s?includeCompensation=false", c.Slug)
	var body struct {
		Jobs []struct {
			Title            string `json:"title"`
			Location         string `json:"location"`
			JobURL           string `json:"jobUrl"`
			IsRemote         bool   `json:"isRemote"`
			DescriptionPlain string `json:"descriptionPlain"`
			PublishedAt      string `json:"publishedAt"`
		} `json:"jobs"`
	}
	if err := httpGetJSON(client, url, &body); err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(body.Jobs))
	for _, j := range body.Jobs {
		posted, _ := time.Parse(time.RFC3339, j.PublishedAt)
		jobs = append(jobs, Job{
			ID:          j.JobURL,
			Company:     c.Name,
			Title:       j.Title,
			Location:    j.Location,
			URL:         j.JobURL,
			Source:      "ashby",
			Remote:      j.IsRemote,
			Snippet:     preview(j.DescriptionPlain),
			PostedAt:    posted,
			FetchedAt:   time.Now().UTC(),
			Description: j.DescriptionPlain,
		})
	}
	return jobs, nil
}

// stripTags removes HTML tags crudely — good enough for keyword matching.
func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}
