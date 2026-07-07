package main

import "testing"

func TestParseYears(t *testing.T) {
	cases := []struct {
		title, desc string
		want        int
	}{
		{"Senior Go Engineer (5+ yrs)", "", 5},
		{"Backend Engineer", "You have 3-5 years of experience with Go.", 3},
		{"Platform Engineer", "Minimum of 4 years building distributed systems.", 4},
		{"SRE", "At least 7 years in infrastructure roles.", 7},
		{"Engineer", "8+ years of professional software development experience.", 8},
		{"Engineer", "3 to 6 years experience preferred", 3},
		{"Engineer", "We were founded 10 years ago and love Go.", 0}, // prose, not a requirement
		{"Engineer", "Great benefits including 401k.", 0},            // nothing
		{"New Grad Software Engineer", "0-2 years experience", 0},    // low end is 0
		{"Staff Engineer", "10+ years of experience required", 10},
	}
	for _, c := range cases {
		if got := parseYears(c.title, c.desc); got != c.want {
			t.Errorf("parseYears(%q, %q) = %d, want %d", c.title, c.desc, got, c.want)
		}
	}
}
