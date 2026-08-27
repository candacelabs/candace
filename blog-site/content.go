package main

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"gopkg.in/yaml.v3"
)

//go:embed content/*.md
var contentFS embed.FS

//go:embed assets/blog.css
var themeCSS string

//go:embed templates/*.html
var templateFS embed.FS

// highlightStyle is the chroma style the generated code-block CSS is built
// from; it must read well on the near-black theme background.
const highlightStyle = "github-dark"

const homeSource = "content/home.md"

var markdown = goldmark.New(
	goldmark.WithExtensions(
		extension.GFM,
		highlighting.NewHighlighting(
			highlighting.WithStyle(highlightStyle),
			highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
		),
	),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
)

type frontmatter struct {
	Title       string    `yaml:"title"`
	Date        time.Time `yaml:"date"`
	Lastmod     time.Time `yaml:"lastmod"`
	Description string    `yaml:"description"`
	Tags        []string  `yaml:"tags"`
	Aliases     []string  `yaml:"aliases"`
}

// LoadPosts renders every embedded post, newest first.
func LoadPosts() ([]Post, error) {
	entries, err := contentFS.ReadDir("content")
	if err != nil {
		return nil, fmt.Errorf("reading embedded content: %w", err)
	}

	var posts []Post
	for _, entry := range entries {
		name := "content/" + entry.Name()
		if name == homeSource || !strings.HasSuffix(name, ".md") {
			continue
		}
		post, err := loadPost(name)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", name, err)
		}
		posts = append(posts, post)
	}

	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Date.After(posts[j].Date)
	})
	return posts, nil
}

// LoadHome renders the embedded home-page prose section.
func LoadHome() (template.HTML, error) {
	raw, err := contentFS.ReadFile(homeSource)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", homeSource, err)
	}
	return renderMarkdown(raw)
}

// LoadTemplates parses the embedded page templates.
func LoadTemplates() (*template.Template, error) {
	return template.ParseFS(templateFS, "templates/*.html")
}

// SiteCSS returns the theme stylesheet merged with the stylesheet for the
// classed chroma spans emitted inside rendered code blocks.
func SiteCSS() (string, error) {
	style := styles.Get(highlightStyle)
	if style == nil {
		return "", fmt.Errorf("unknown chroma style %q", highlightStyle)
	}
	var buf bytes.Buffer
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	if err := formatter.WriteCSS(&buf, style); err != nil {
		return "", fmt.Errorf("writing chroma css: %w", err)
	}
	return themeCSS + "\n" + buf.String(), nil
}

func loadPost(path string) (Post, error) {
	raw, err := contentFS.ReadFile(path)
	if err != nil {
		return Post{}, err
	}

	meta, body, err := splitFrontmatter(raw)
	if err != nil {
		return Post{}, err
	}
	if meta.Title == "" {
		return Post{}, fmt.Errorf("missing title in frontmatter")
	}
	if meta.Date.IsZero() {
		return Post{}, fmt.Errorf("missing date in frontmatter")
	}

	content, err := renderMarkdown(body)
	if err != nil {
		return Post{}, err
	}

	slug := strings.TrimSuffix(strings.TrimPrefix(path, "content/"), ".md")
	return Post{
		Slug:        slug,
		Title:       meta.Title,
		Date:        meta.Date,
		Lastmod:     meta.Lastmod,
		Description: meta.Description,
		Tags:        meta.Tags,
		Aliases:     meta.Aliases,
		Content:     content,
	}, nil
}

func splitFrontmatter(raw []byte) (frontmatter, []byte, error) {
	var meta frontmatter
	const delim = "---"

	text := string(raw)
	if !strings.HasPrefix(text, delim+"\n") {
		return meta, nil, fmt.Errorf("missing frontmatter delimiter")
	}
	rest := text[len(delim)+1:]
	end := strings.Index(rest, "\n"+delim+"\n")
	if end < 0 {
		return meta, nil, fmt.Errorf("unterminated frontmatter")
	}

	if err := yaml.Unmarshal([]byte(rest[:end]), &meta); err != nil {
		return meta, nil, fmt.Errorf("parsing frontmatter: %w", err)
	}
	return meta, []byte(rest[end+len(delim)+2:]), nil
}

func renderMarkdown(source []byte) (template.HTML, error) {
	var buf bytes.Buffer
	if err := markdown.Convert(source, &buf); err != nil {
		return "", fmt.Errorf("rendering markdown: %w", err)
	}
	// The converted output comes from trusted, reviewed content in this
	// repository, not from user input.
	return template.HTML(buf.String()), nil
}
