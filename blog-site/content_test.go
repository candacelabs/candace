package main

import (
	"strings"
	"testing"
)

func TestLoadPostsRendersNewestFirst(t *testing.T) {
	posts, err := LoadPosts()
	if err != nil {
		t.Fatalf("LoadPosts: %v", err)
	}
	if len(posts) < 2 {
		t.Fatalf("expected at least 2 posts, got %d", len(posts))
	}
	for i := 1; i < len(posts); i++ {
		if posts[i].Date.After(posts[i-1].Date) {
			t.Errorf("posts out of order: %s after %s", posts[i].Slug, posts[i-1].Slug)
		}
	}
}

func TestLoadPostsCarriesFrontmatterAndMarkdown(t *testing.T) {
	posts, err := LoadPosts()
	if err != nil {
		t.Fatalf("LoadPosts: %v", err)
	}
	var post *Post
	for i := range posts {
		if posts[i].Slug == "why-static" {
			post = &posts[i]
		}
	}
	if post == nil {
		t.Fatal("why-static post not found")
	}

	checks := []struct {
		name string
		got  bool
	}{
		{"title", post.Title == "Why this blog stays static"},
		{"date", post.DateISO() == "2025-08-24"},
		{"alias", len(post.Aliases) == 1 && post.Aliases[0] == "/posts/first-post/"},
		{"permalink", post.Permalink() == "/posts/why-static/"},
		{"heading id", strings.Contains(string(post.Content), `id="less-machinery"`)},
		{"highlighted code", strings.Contains(string(post.Content), "chroma")},
	}
	for _, check := range checks {
		if !check.got {
			t.Errorf("%s check failed", check.name)
		}
	}
}

func TestLoadHomeAndCSS(t *testing.T) {
	home, err := LoadHome()
	if err != nil {
		t.Fatalf("LoadHome: %v", err)
	}
	if !strings.Contains(string(home), "engineering notebook") {
		t.Error("home prose missing expected copy")
	}

	css, err := SiteCSS()
	if err != nil {
		t.Fatalf("SiteCSS: %v", err)
	}
	if !strings.Contains(css, "--accent") || !strings.Contains(css, ".chroma") {
		t.Error("stylesheet missing theme or chroma rules")
	}
}
