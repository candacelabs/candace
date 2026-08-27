package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func renderedSite(t *testing.T, site Site) string {
	t.Helper()
	out := t.TempDir()
	if err := RenderSite(out, site); err != nil {
		t.Fatalf("RenderSite: %v", err)
	}
	return out
}

func readPage(t *testing.T, out, relPath string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(out, relPath))
	if err != nil {
		t.Fatalf("reading %s: %v", relPath, err)
	}
	return string(raw)
}

func TestRenderSiteWritesEveryPage(t *testing.T) {
	site := DefaultSite()
	site.ReleaseSHA = "0123456789abcdef0123456789abcdef01234567"
	site.Projects = []Project{{
		Name:        "copilot-pair",
		Description: "Share one Copilot CLI session with every connected browser.",
		URL:         "https://github.com/candacelabs/copilot-pair",
	}}
	out := renderedSite(t, site)

	for _, path := range []string{
		"index.html",
		"posts/index.html",
		"posts/why-static/index.html",
		"posts/terminal-aesthetic/index.html",
		"posts/first-post/index.html",
		"404.html",
		"index.xml",
		"assets/blog.css",
		"robots.txt",
		"CNAME",
	} {
		if _, err := os.Stat(filepath.Join(out, path)); err != nil {
			t.Errorf("missing %s: %v", path, err)
		}
	}

	home := readPage(t, out, "index.html")
	for _, want := range []string{
		"Candace Labs",
		"copilot-pair",
		"Why this blog stays static",
		`name="candace-release"`,
		site.ReleaseSHA,
	} {
		if !strings.Contains(home, want) {
			t.Errorf("homepage missing %q", want)
		}
	}
	if strings.Contains(home, "<script") {
		t.Error("homepage must not include scripts")
	}

	stub := readPage(t, out, "posts/first-post/index.html")
	if !strings.Contains(stub, "url=/posts/why-static/") {
		t.Error("alias stub does not redirect to the canonical note")
	}

	if got := strings.TrimSpace(readPage(t, out, "CNAME")); got != "blog.candace.cloud" {
		t.Errorf("CNAME = %q", got)
	}
}

func TestRenderSiteOmitsReleaseAndGridWhenAbsent(t *testing.T) {
	out := renderedSite(t, DefaultSite())

	home := readPage(t, out, "index.html")
	if strings.Contains(home, "candace-release") {
		t.Error("release meta must be absent without a release SHA")
	}
	if strings.Contains(home, "Current projects") {
		t.Error("projects section must be absent without derived projects")
	}
}

func TestRenderedFeedParses(t *testing.T) {
	out := renderedSite(t, DefaultSite())

	var feed struct {
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Link string `xml:"link"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal([]byte(readPage(t, out, "index.xml")), &feed); err != nil {
		t.Fatalf("parsing feed: %v", err)
	}
	if feed.Channel.Title != "Candace Labs" {
		t.Errorf("feed title = %q", feed.Channel.Title)
	}
	if len(feed.Channel.Items) < 2 {
		t.Errorf("expected at least 2 feed items, got %d", len(feed.Channel.Items))
	}
	for _, item := range feed.Channel.Items {
		if !strings.HasPrefix(item.Link, "https://blog.candace.cloud/posts/") {
			t.Errorf("unexpected feed link %q", item.Link)
		}
	}
}
