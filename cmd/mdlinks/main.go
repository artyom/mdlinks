package main

import (
	"errors"
	"flag"
	"log"
	"os"

	"github.com/artyom/mdlinks"
)

func main() {
	log.SetFlags(0)
	dir := "."
	pat := "*.md"
	flag.StringVar(&dir, "dir", dir, "`directory` to scan; it's considered to be a root for absolute links")
	flag.StringVar(&pat, "pat", pat, "glob `pattern` to match markdown files")
	flag.Parse()
	err := mdlinks.CheckFS(os.DirFS(dir), pat)
	var e *mdlinks.BrokenLinksError
	if errors.As(err, &e) {
		isGithub := os.Getenv("GITHUB_ACTIONS") == "true"
		for _, l := range e.Links {
			log.Println(l)
			if isGithub {
				// https://docs.github.com/en/actions/learn-github-actions/workflow-commands-for-github-actions#setting-an-error-message
				// ::error file={name},line={line},endLine={endLine},title={title}::{message}
				switch l.Link.LineStart {
				case 0:
					log.Printf("::error file=%s,title=%s::%s", l.File, l.Reason(), l)
				default:
					log.Printf("::error file=%s,line=%d,endLine=%d,title=%s::%s",
						l.File, l.Link.LineStart, l.Link.LineEnd, l.Reason(), l)
				}
			}
		}
		os.Exit(127)
	}
	if err != nil {
		log.Fatal(err)
	}
}
