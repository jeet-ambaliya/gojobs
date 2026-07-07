package main

import "time"

// Company is one entry in companies.json — a company plus which ATS
// (applicant tracking system) hosts its public job board and the slug
// that identifies it on that ATS.
type Company struct {
	Name string `json:"name"`
	ATS  string `json:"ats"`  // "greenhouse" | "lever" | "ashby"
	Slug string `json:"slug"` // the board token in the ATS URL
}

// Job is a single normalized posting. Fields are populated from whichever
// ATS it came from and then filtered/stored.
type Job struct {
	ID          string    `json:"id"` // we use the posting URL — unique and stable
	Company     string    `json:"company"`
	Title       string    `json:"title"`
	Location    string    `json:"location"`
	URL         string    `json:"url"`
	Source      string    `json:"source"` // which ATS it came from
	Remote      bool      `json:"remote"`
	MinYears    int       `json:"min_years,omitempty"` // min years of experience named in the posting (0 = unspecified)
	Snippet     string    `json:"snippet,omitempty"`   // short description preview
	PostedAt    time.Time `json:"posted_at,omitempty"`
	FetchedAt   time.Time `json:"fetched_at"`
	Description string    `json:"-"` // full text, used only for matching, never persisted
}
