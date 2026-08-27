package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func repoListingServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	fixture, err := os.ReadFile("testdata/github_repos.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/orgs/candacelabs/repos" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write(fixture)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestFetchProjectsFiltersSortsAndCaps(t *testing.T) {
	server := repoListingServer(t, http.StatusOK)

	projects, err := FetchProjects(server.Client(), server.URL, "candacelabs", "blog-site", "")
	if err != nil {
		t.Fatalf("FetchProjects: %v", err)
	}

	if len(projects) != maxProjects {
		t.Fatalf("expected %d projects, got %d", maxProjects, len(projects))
	}
	if projects[0].Name != "copilot-pair" {
		t.Errorf("expected most recently pushed first, got %s", projects[0].Name)
	}
	for _, project := range projects {
		switch project.Name {
		case "blog-site", "some-fork", "old-experiment", "older-b":
			t.Errorf("%s must not appear in the grid", project.Name)
		case "no-description":
			if project.Description != fallbackDescription {
				t.Errorf("expected fallback description, got %q", project.Description)
			}
		}
	}
}

func TestFetchProjectsFailsLoudlyOnAPIError(t *testing.T) {
	server := repoListingServer(t, http.StatusForbidden)

	if _, err := FetchProjects(server.Client(), server.URL, "candacelabs", "blog-site", ""); err == nil {
		t.Fatal("expected an error on non-200 response")
	}
}
