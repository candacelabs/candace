package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"
)

// The projects grid is derived, never hand-edited: at render time the site
// lists the org's public repositories, so every publish reflects what the
// org actually ships.

const fallbackDescription = "A candacelabs project."

// maxProjects caps the grid so the home page stays a selection, not an index.
const maxProjects = 6

type githubRepo struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	HTMLURL     string    `json:"html_url"`
	Fork        bool      `json:"fork"`
	Archived    bool      `json:"archived"`
	PushedAt    time.Time `json:"pushed_at"`
}

// FetchProjects derives the grid from the org's public repositories,
// most recently pushed first. The site's own repository is excluded.
func FetchProjects(client *http.Client, apiBase, org, ownRepo, token string) ([]Project, error) {
	url := fmt.Sprintf("%s/orgs/%s/repos?type=public&per_page=100", apiBase, org)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building repo listing request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing %s repositories: %w", org, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("listing %s repositories: %s: %s", org, resp.Status, body)
	}

	var repos []githubRepo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("decoding repo listing: %w", err)
	}

	sort.Slice(repos, func(i, j int) bool {
		return repos[i].PushedAt.After(repos[j].PushedAt)
	})

	var projects []Project
	for _, repo := range repos {
		if repo.Fork || repo.Archived || repo.Name == ownRepo {
			continue
		}
		description := repo.Description
		if description == "" {
			description = fallbackDescription
		}
		projects = append(projects, Project{
			Name:        repo.Name,
			Description: description,
			URL:         repo.HTMLURL,
		})
		if len(projects) == maxProjects {
			break
		}
	}
	return projects, nil
}
