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
	flag.StringVar(&dir, "d", dir, "directory to scan; it's considered to be a root for absolute links")
	flag.StringVar(&pat, "p", pat, "glob pattern to match markdown files")
	flag.Parse()
	err := mdlinks.CheckFS(os.DirFS(dir), pat)
	var e *mdlinks.BrokenLinksError
	if errors.As(err, &e) {
		for _, link := range e.Links {
			log.Println(link)
		}
		os.Exit(127)
	}
	if err != nil {
		log.Fatal(err)
	}
}
