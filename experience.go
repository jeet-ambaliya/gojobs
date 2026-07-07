package main

import (
	"regexp"
	"strconv"
)

// Experience parsing: pull the minimum years of experience a posting asks for
// out of its free text. This is a best-effort heuristic — job descriptions
// state requirements a hundred different ways — so it aims for the common,
// unambiguous phrasings and returns 0 ("unspecified") when nothing clearly
// matches, rather than guessing.

var (
	// "3-5 years", "3 to 5 years", "3–5 yrs" → take the low end (3).
	yearsRangeRe = regexp.MustCompile(`(?i)\b(\d{1,2})\s*(?:-|–|to)\s*(\d{1,2})\s*\+?\s*(?:years?|yrs?)\b`)
	// "5+ years", "5 + yrs" → 5.
	yearsPlusRe = regexp.MustCompile(`(?i)\b(\d{1,2})\s*\+\s*(?:years?|yrs?)\b`)
	// "at least 4 years", "minimum of 4 years", "min. 4 yrs" → 4.
	yearsAtLeastRe = regexp.MustCompile(`(?i)\b(?:at least|minimum(?:\s+of)?|min\.?)\s+(\d{1,2})\s*(?:years?|yrs?)\b`)
	// "5 years of experience", "5 years' experience", "5 years relevant experience".
	yearsExpRe = regexp.MustCompile(`(?i)\b(\d{1,2})\s*(?:\+)?\s*(?:years?|yrs?)[^.]{0,25}?experience\b`)
)

// parseYears returns the minimum years of experience named in the posting, or 0
// if none is clearly stated. Title is checked first (e.g. "Senior Go Engineer
// (5+ yrs)"), then the description.
func parseYears(title, desc string) int {
	for _, text := range []string{title, desc} {
		if y := yearsFromText(text); y > 0 {
			return y
		}
	}
	return 0
}

// yearsFromText tries each pattern in priority order — most specific first —
// and returns the value from the first that matches. A range yields its low end
// (the minimum requirement), so "0-2 years" is 0, i.e. no real floor. Values
// above 30 are treated as noise (e.g. "over 30 years" in a benefits blurb).
func yearsFromText(text string) int {
	for _, p := range []struct {
		re  *regexp.Regexp
		grp int
	}{
		{yearsRangeRe, 1},   // "3-5 years" → low end
		{yearsPlusRe, 1},    // "5+ years"
		{yearsAtLeastRe, 1}, // "at least 4 years"
		{yearsExpRe, 1},     // "5 years of experience"
	} {
		if m := p.re.FindStringSubmatch(text); m != nil {
			if v := atoi(m[p.grp]); v >= 0 && v <= 30 {
				return v
			}
		}
	}
	return 0
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
