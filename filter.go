package main

import "regexp"

var (
	// "golang" anywhere is an unambiguous signal.
	golangRe = regexp.MustCompile(`(?i)\bgolang\b`)
	// standalone "go" token — noisy in prose, so only trusted inside a title.
	goTokenRe = regexp.MustCompile(`(?i)\bgo\b`)
	// engineering role words that make a bare "Go" in a title trustworthy.
	engRoleRe = regexp.MustCompile(`(?i)\b(engineer|developer|programmer|backend|back-end|sre|platform|infrastructure|software|architect)\b`)

	canadaRe = regexp.MustCompile(`(?i)\b(canada|canadian|toronto|vancouver|montr[eé]al|ottawa|calgary|edmonton|winnipeg|waterloo|kitchener|hamilton|mississauga|burnaby|victoria|halifax|qu[eé]bec|quebec city|ontario|british columbia|alberta|manitoba|saskatchewan|nova scotia|new brunswick|newfoundland|\bon\b|\bqc\b|\bbc\b|\bab\b)\b`)
	remoteRe = regexp.MustCompile(`(?i)\bremote\b`)
)

// matchesGo reports whether a posting is plausibly a Go role.
//
// Rule: "golang" anywhere counts; a bare "Go" only counts when it sits in a
// title next to an engineering role word (e.g. "Senior Go Engineer"). This
// keeps out prose like "ready to go" and "go-to-market".
func matchesGo(title, desc string) bool {
	if golangRe.MatchString(title) || golangRe.MatchString(desc) {
		return true
	}
	if goTokenRe.MatchString(title) && engRoleRe.MatchString(title) {
		return true
	}
	return false
}

// matchesCanada reports whether a posting looks Canada-based, and whether it
// looks remote. A posting can be both (e.g. "Remote — Canada").
func matchesCanada(location, desc string, remoteFlag bool) (canada bool, remote bool) {
	canada = canadaRe.MatchString(location) || canadaRe.MatchString(desc)
	remote = remoteFlag || remoteRe.MatchString(location)
	return canada, remote
}
