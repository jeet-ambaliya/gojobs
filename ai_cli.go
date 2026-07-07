package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// cliClient classifies postings by shelling out to the Claude Code CLI
// (`claude -p`), so runs are covered by a Claude Code subscription instead of
// API credits. Requires a logged-in `claude` on PATH.
type cliClient struct {
	bin     string        // resolved path to the claude executable
	model   string        // optional --model override; empty = your CLI default
	timeout time.Duration // per-batch timeout (CLI startup makes calls slower than the API)
}

// newCLIClient verifies the claude binary exists so a misconfigured run fails
// before anything is fetched.
func newCLIClient(model string) (*cliClient, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found on PATH — install Claude Code, or use -ai with an API key")
	}
	return &cliClient{bin: bin, model: model, timeout: 3 * time.Minute}, nil
}

// name identifies the classifier in log output.
func (c *cliClient) name() string {
	if c.model != "" {
		return "claude CLI (" + c.model + ")"
	}
	return "claude CLI"
}

// classifyBatch pipes one batch prompt to `claude -p` and parses the verdicts.
// The prompt goes via stdin to dodge Windows command-line length limits.
func (c *cliClient) classifyBatch(batch []Job) ([]verdict, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	args := []string{"-p"}
	if c.model != "" {
		args = append(args, "--model", c.model)
	}
	cmd := exec.CommandContext(ctx, c.bin, args...)
	cmd.Stdin = strings.NewReader(aiSystemPrompt + "\n\nPostings:\n" + buildBatchPrompt(batch))
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errOut.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("claude CLI: %s", msg)
	}
	return parseVerdicts(out.String())
}
