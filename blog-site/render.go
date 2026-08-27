package main

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const aliasStub = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta http-equiv="refresh" content="0; url=%[1]s">
<link rel="canonical" href="%[1]s">
<title>Redirecting</title>
</head>
<body><p>This note moved to <a href="%[1]s">%[1]s</a>.</p></body>
</html>
`

// RenderSite writes the complete static site into outDir.
func RenderSite(outDir string, site Site) error {
	posts, err := LoadPosts()
	if err != nil {
		return fmt.Errorf("loading posts: %w", err)
	}
	home, err := LoadHome()
	if err != nil {
		return fmt.Errorf("loading home page: %w", err)
	}
	templates, err := LoadTemplates()
	if err != nil {
		return fmt.Errorf("parsing templates: %w", err)
	}
	css, err := SiteCSS()
	if err != nil {
		return fmt.Errorf("building stylesheet: %w", err)
	}

	recent := posts
	if len(recent) > 5 {
		recent = recent[:5]
	}

	pages := []struct {
		path     string
		template string
		data     PageData
	}{
		{"index.html", "home.html", PageData{
			Site: site, Nav: "home",
			PageTitle:       site.Title,
			PageDescription: site.Description,
			Posts:           recent, Home: home,
		}},
		{"posts/index.html", "list.html", PageData{
			Site: site, Nav: "posts",
			PageTitle:       "Notes · " + site.Title,
			PageDescription: site.Description,
			Posts:           posts,
		}},
		{"404.html", "error.html", PageData{
			Site:            site,
			PageTitle:       "Not found · " + site.Title,
			PageDescription: site.Description,
		}},
	}
	for i := range posts {
		post := posts[i]
		pages = append(pages, struct {
			path     string
			template string
			data     PageData
		}{
			filepath.Join("posts", post.Slug, "index.html"), "post.html", PageData{
				Site: site, Nav: "posts",
				PageTitle:       post.Title + " · " + site.Title,
				PageDescription: post.Description,
				Post:            &post,
			},
		})
	}

	for _, page := range pages {
		var buf bytes.Buffer
		if err := templates.ExecuteTemplate(&buf, page.template, page.data); err != nil {
			return fmt.Errorf("rendering %s: %w", page.path, err)
		}
		if err := writeFile(outDir, page.path, buf.Bytes()); err != nil {
			return err
		}
	}

	for _, post := range posts {
		for _, alias := range post.Aliases {
			stubPath, err := aliasStubPath(alias)
			if err != nil {
				return fmt.Errorf("post %s: %w", post.Slug, err)
			}
			stub := fmt.Sprintf(aliasStub, template.HTMLEscapeString(post.Permalink()))
			if err := writeFile(outDir, stubPath, []byte(stub)); err != nil {
				return err
			}
		}
	}

	rss, err := buildRSS(site, posts)
	if err != nil {
		return fmt.Errorf("building rss feed: %w", err)
	}
	domain, err := siteDomain(site.BaseURL)
	if err != nil {
		return err
	}

	static := map[string][]byte{
		"index.xml":       rss,
		"assets/blog.css": []byte(css),
		"robots.txt":      []byte("User-agent: *\nDisallow:\n"),
		"CNAME":           []byte(domain + "\n"),
	}
	for path, content := range static {
		if err := writeFile(outDir, path, content); err != nil {
			return err
		}
	}
	return nil
}

func writeFile(outDir, relPath string, content []byte) error {
	path := filepath.Join(outDir, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", relPath, err)
	}
	return nil
}

// aliasStubPath maps a Hugo-era alias URL onto the static file that serves
// its redirect stub.
func aliasStubPath(alias string) (string, error) {
	trimmed := strings.Trim(alias, "/")
	if trimmed == "" || strings.Contains(trimmed, "..") {
		return "", fmt.Errorf("invalid alias %q", alias)
	}
	return filepath.Join(filepath.FromSlash(trimmed), "index.html"), nil
}

func siteDomain(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("invalid base url %q", baseURL)
	}
	return parsed.Hostname(), nil
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Description   string    `xml:"description"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Items         []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

func buildRSS(site Site, posts []Post) ([]byte, error) {
	base := strings.TrimSuffix(site.BaseURL, "/")
	channel := rssChannel{
		Title:       site.Title,
		Link:        site.BaseURL,
		Description: site.Description,
	}
	for _, post := range posts {
		link := base + post.Permalink()
		channel.Items = append(channel.Items, rssItem{
			Title:       post.Title,
			Link:        link,
			GUID:        link,
			PubDate:     post.Date.Format(http.TimeFormat),
			Description: post.Description,
		})
	}
	if len(posts) > 0 {
		channel.LastBuildDate = posts[0].Date.Format(http.TimeFormat)
	}

	body, err := xml.MarshalIndent(rssFeed{Version: "2.0", Channel: channel}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), body...), nil
}
