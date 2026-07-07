package main
import "testing"
func TestMatchesGo(t *testing.T){
  cases := []struct{ title, desc string; want bool }{
    {"Senior Go Engineer", "", true},
    {"Backend Developer (Golang)", "", true},
    {"Software Engineer", "We build services in Golang and Rust.", true},
    {"Go-to-Market Manager", "own the go-to-market motion", false},
    {"Product Designer", "ready to go fast", false},
    {"Senior Go Developer", "", true},                // Go token + role word in title
    {"Backend Engineer", "some go code here", false}, // bare "go" in desc is intentionally ignored
    {"Sales Lead", "", false},
  }
  for _, c := range cases {
    if got := matchesGo(c.title, c.desc); got != c.want {
      t.Errorf("matchesGo(%q,%q)=%v want %v", c.title, c.desc, got, c.want)
    }
  }
}
func TestMatchesCanada(t *testing.T){
  ca, rem := matchesCanada("Toronto, ON", "", false)
  if !ca || rem { t.Errorf("Toronto: ca=%v rem=%v", ca, rem) }
  ca, rem = matchesCanada("Remote - Canada", "", false)
  if !ca || !rem { t.Errorf("Remote CA: ca=%v rem=%v", ca, rem) }
  ca, rem = matchesCanada("Remote", "", true)
  if ca || !rem { t.Errorf("Remote only: ca=%v rem=%v", ca, rem) }
  ca, _ = matchesCanada("Berlin, Germany", "", false)
  if ca { t.Errorf("Berlin should not be Canada") }
}
