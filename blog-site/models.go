package main

import (
	"html/template"
	"time"
)

// Post is one markdown note, rendered to HTML at load time.
type Post struct {
	Slug        string
	Title       string
	Date        time.Time
	Lastmod     time.Time
	Description string
	Tags        []string
	Aliases     []string
	Content     template.HTML
}

// DateISO formats the publication date for datetime attributes.
func (p Post) DateISO() string {
	return p.Date.Format("2006-01-02")
}

// DateHuman formats the publication date for display.
func (p Post) DateHuman() string {
	return p.Date.Format("Jan 2, 2006")
}

// LastmodHuman formats the last-modified date for display; empty when the
// post was never updated after publication.
func (p Post) LastmodHuman() string {
	if p.Lastmod.IsZero() || p.Lastmod.Equal(p.Date) {
		return ""
	}
	return p.Lastmod.Format("Jan 2, 2006")
}

// Permalink is the site-relative canonical path for the post. Trailing-slash
// paths keep the URLs the Hugo site published.
func (p Post) Permalink() string {
	return "/posts/" + p.Slug + "/"
}

// Project is one entry in the projects grid on the home page, derived from
// the GitHub API at render time — never hand-edited.
type Project struct {
	Name        string
	Description string
	URL         string
}

// Site carries the site-wide configuration shared by every page.
type Site struct {
	Title       string
	BaseURL     string
	Description string
	Eyebrow     string
	Org         string
	GitHubURL   string
	Projects    []Project
	// ReleaseSHA is the published revision; when set, every page carries a
	// candace-release meta tag.
	ReleaseSHA string
}

// DefaultSite mirrors the parameters the site has always shipped with.
func DefaultSite() Site {
	return Site{
		Title:       "Candace Labs",
		BaseURL:     "https://blog.candace.cloud/",
		Description: "Field notes on small systems, useful tools, and software with fewer moving parts.",
		Eyebrow:     "systems / tools / field notes",
		Org:         "candacelabs",
		GitHubURL:   "https://github.com/candacelabs",
	}
}

// PageData is the payload every template renders from.
type PageData struct {
	Site            Site
	Nav             string
	PageTitle       string
	PageDescription string
	Posts           []Post
	Post            *Post
	Home            template.HTML
}
