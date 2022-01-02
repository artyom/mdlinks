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
				log.Printf("::error file=%s,title=%s::%s", l.File, l.Reason(), l)
			}
		}
		os.Exit(127)
	}
	if err != nil {
		log.Fatal(err)
	}
}
