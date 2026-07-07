package main

import (
	"encoding/json"
	"os"
	"sort"
)

func loadCompanies(path string) ([]Company, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cs []Company
	if err := json.Unmarshal(b, &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

// loadCompaniesQuiet returns an empty slice if the file is missing or unreadable.
func loadCompaniesQuiet(path string) []Company {
	cs, err := loadCompanies(path)
	if err != nil {
		return nil
	}
	return cs
}

// saveCompanies writes the company list back as pretty JSON.
func saveCompanies(path string, cs []Company) error {
	b, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func loadJobs(path string) ([]Job, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var js []Job
	if err := json.Unmarshal(b, &js); err != nil {
		return nil, err
	}
	return js, nil
}

// loadJobsQuiet returns an empty slice if the file is missing or unreadable.
func loadJobsQuiet(path string) []Job {
	js, err := loadJobs(path)
	if err != nil {
		return nil
	}
	return js
}

// mergeJobs combines existing and fresh jobs, deduped by ID (the posting URL).
// Fresh entries win so PostedAt/Snippet stay current. Results are sorted
// newest-first by posting date, then company name.
func mergeJobs(existing, fresh []Job) []Job {
	byID := make(map[string]Job, len(existing)+len(fresh))
	for _, j := range existing {
		byID[j.ID] = j
	}
	for _, j := range fresh {
		byID[j.ID] = j // overwrite stale copy
	}
	out := make([]Job, 0, len(byID))
	for _, j := range byID {
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool {
		if !out[i].PostedAt.Equal(out[k].PostedAt) {
			return out[i].PostedAt.After(out[k].PostedAt)
		}
		return out[i].Company < out[k].Company
	})
	return out
}

func saveJobs(path string, jobs []Job) error {
	b, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
