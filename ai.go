package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// This file adds an optional Claude-backed relevance filter. Instead of the
// keyword regexes in filter.go, it hands each posting's title/location/snippet
// to Claude and asks whether it's a genuine Go software-engineering role in
// Canada. It talks to the Messages API directly over net/http to keep the
// project standard-library-only (no SDK dependency), mirroring ats.go.

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	defaultAIModel   = "claude-opus-4-8" // override with -model (e.g. claude-haiku-4-5 for a cheaper run)
	aiBatchSize      = 25                // postings per request — keeps prompts small and cache-friendly
)

// aiClient calls the Anthropic Messages API to classify postings.
type aiClient struct {
	http   *http.Client
	apiKey string
	model  string
}

// newAIClient reads the API key from ANTHROPIC_API_KEY. It errors early (before
// any fetching) if the key is missing, so a misconfigured run fails fast.
func newAIClient(model string, timeout time.Duration) (*aiClient, error) {
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set — export it, or drop -ai to use the keyword filter")
	}
	if model == "" {
		model = defaultAIModel
	}
	return &aiClient{http: &http.Client{Timeout: timeout}, apiKey: key, model: model}, nil
}

// verdict is Claude's judgment for one posting, keyed back by its batch index.
type verdict struct {
	Index    int  `json:"index"`
	GoRole   bool `json:"go_role"`
	InCanada bool `json:"in_canada"`
	Remote   bool `json:"remote"`
}

// batchClassifier is anything that can judge one batch of postings — the
// Messages API client (-ai) or the Claude Code CLI (-ai-cli).
type batchClassifier interface {
	classifyBatch(batch []Job) ([]verdict, error)
	name() string
}

const aiSystemPrompt = `You classify job postings for a tool that surfaces Go/Golang software-engineering roles based in Canada. For each posting decide three booleans:

- go_role: true only if it is a software-engineering position (backend, platform, infrastructure, SRE, full-stack, etc.) where Go/Golang is a primary or explicitly listed language. A bare "Go" in prose ("go-to-market", "on the go") does NOT count. Non-engineering roles (sales, marketing, recruiting, support, management-only) are never go_role, even if they mention Go.
- in_canada: true if the role's LOCATION is in Canada (a Canadian city or province), or it is explicitly remote within Canada. A description that merely mentions a Canadian office while the role's location is elsewhere (e.g. "United States", "London") is NOT in_canada.
- remote: true if the role is remote.

Reply with ONLY a JSON array, one object per posting, each {"index":N,"go_role":bool,"in_canada":bool,"remote":bool}. No prose, no markdown fences.`

// name identifies the classifier in log output.
func (a *aiClient) name() string { return a.model }

// classifyJobs runs every posting through the classifier in batches and keeps
// the ones that are Go roles in Canada (or remote, when includeRemote is set).
// It sets each kept job's Remote flag from Claude's judgment. Batches that
// error out are logged and skipped so one bad batch never sinks the whole run.
func classifyJobs(cl batchClassifier, jobs []Job, includeRemote bool, log func(string, ...any)) []Job {
	var kept []Job
	for start := 0; start < len(jobs); start += aiBatchSize {
		end := start + aiBatchSize
		if end > len(jobs) {
			end = len(jobs)
		}
		batch := jobs[start:end]

		verdicts, err := cl.classifyBatch(batch)
		if err != nil {
			log("  ! AI batch %d-%d failed: %v (skipped)", start, end-1, err)
			continue
		}
		for _, v := range verdicts {
			if v.Index < 0 || v.Index >= len(batch) {
				continue
			}
			if !v.GoRole || !(v.InCanada || (includeRemote && v.Remote)) {
				continue
			}
			j := batch[v.Index]
			j.Remote = v.Remote
			j.Description = "" // don't persist full text (matches the regex path)
			kept = append(kept, j)
		}
	}
	return kept
}

// buildBatchPrompt renders one line per posting for the classifier prompt.
func buildBatchPrompt(batch []Job) string {
	var b strings.Builder
	for i, j := range batch {
		loc := j.Location
		if loc == "" {
			loc = "(unstated)"
		}
		fmt.Fprintf(&b, "[%d] %s | %s | %s | %s\n", i, j.Company, j.Title, loc, j.Snippet)
	}
	return b.String()
}

// parseVerdicts pulls the JSON verdict array out of the model's reply text.
func parseVerdicts(text string) ([]verdict, error) {
	arr := extractJSONArray(text)
	if arr == "" {
		return nil, fmt.Errorf("no JSON array in response")
	}
	var verdicts []verdict
	if err := json.Unmarshal([]byte(arr), &verdicts); err != nil {
		return nil, fmt.Errorf("parse verdicts: %w", err)
	}
	return verdicts, nil
}

// classifyBatch sends one batch and parses Claude's JSON verdicts.
func (a *aiClient) classifyBatch(batch []Job) ([]verdict, error) {
	reqBody := map[string]any{
		"model":      a.model,
		"max_tokens": 4096,
		"system":     aiSystemPrompt,
		"messages": []map[string]any{
			{"role": "user", "content": buildBatchPrompt(batch)},
		},
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, anthropicURL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := a.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Error      *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		if body.Error != nil {
			return nil, fmt.Errorf("http %d: %s", resp.StatusCode, body.Error.Message)
		}
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	if body.StopReason == "refusal" {
		return nil, fmt.Errorf("model refused the request")
	}

	// Concatenate text blocks and pull out the JSON array.
	var text strings.Builder
	for _, c := range body.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}
	return parseVerdicts(text.String())
}

// extractJSONArray returns the substring from the first '[' to the last ']',
// tolerating any stray prose the model might wrap around the JSON.
func extractJSONArray(s string) string {
	i := strings.IndexByte(s, '[')
	j := strings.LastIndexByte(s, ']')
	if i < 0 || j < 0 || j < i {
		return ""
	}
	return s[i : j+1]
}
