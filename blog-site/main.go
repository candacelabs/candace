// Command blog-site renders the Candace Labs blog as a static site. The
// canonical source lives in the candace-server monorepo; the exported
// snapshot's Pages workflow runs `render` on every push of its main branch.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) < 2 {
		log.Fatal("usage: blog-site render|serve [flags]")
	}

	switch os.Args[1] {
	case "render":
		flags := flag.NewFlagSet("render", flag.ExitOnError)
		out := flags.String("out", "public", "output directory")
		offline := flags.Bool("offline", false, "skip the GitHub project listing")
		_ = flags.Parse(os.Args[2:])
		if err := run(*out, *offline); err != nil {
			log.Fatal(err)
		}
	case "serve":
		flags := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := flags.String("addr", "127.0.0.1:1313", "listen address")
		offline := flags.Bool("offline", true, "skip the GitHub project listing")
		_ = flags.Parse(os.Args[2:])
		dir, err := os.MkdirTemp("", "blog-site-*")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(dir)
		if err := run(dir, *offline); err != nil {
			log.Fatal(err)
		}
		log.Printf("serving on http://%s", *addr)
		log.Fatal(http.ListenAndServe(*addr, http.FileServer(http.Dir(dir))))
	default:
		log.Fatalf("unknown subcommand %q", os.Args[1])
	}
}

func run(outDir string, offline bool) error {
	site := DefaultSite()
	site.ReleaseSHA = os.Getenv("RELEASE_SHA")

	if !offline {
		projects, err := FetchProjects(
			http.DefaultClient,
			"https://api.github.com",
			site.Org,
			"blog-site",
			os.Getenv("GITHUB_TOKEN"),
		)
		if err != nil {
			return fmt.Errorf("deriving projects: %w", err)
		}
		site.Projects = projects
	}

	if err := RenderSite(outDir, site); err != nil {
		return err
	}
	log.Printf("rendered site into %s (%d projects)", outDir, len(site.Projects))
	return nil
}
