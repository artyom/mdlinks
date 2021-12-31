// Package mdurlcheck provides functions to verify cross links in a set of
// markdown files.
package mdurlcheck

import (
	"io/fs"
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// CheckFS walks file system fsys looking for files with their base names
// matching pattern pat. It parses such files as markdown, looks for local urls
// (urls that only specify paths), and reports if it find any urls that point
// to non-existing files.
//
// If returned error is a *BrokenLinksError, it describes found files with
// broken links.
func CheckFS(fsys fs.FS, pat string) error {
	if _, err := path.Match(pat, "xxx"); err != nil { // report bad pattern early
		return err
	}
	exists := func(p string) bool {
		f, err := fsys.Open(p)
		if err != nil {
			return false
		}
		defer f.Close()
		return true
	}
	var brokenLinks []BrokenLink
	fn := func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ok, _ := path.Match(pat, d.Name()); !ok {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		links, err := extractLocalLinks(b)
		if err != nil {
			return err
		}
		// log.Printf("DEBUG: %s: %v", p, links)
		for _, s := range links {
			var srel string // fs.FS relative path that link points to

			if s != "" && s[0] == '/' { // e.g. “/abc”
				srel = s[1:]
			} else { // e.g. “abc” or “../abc”
				srel = path.Join(strings.TrimSuffix(p, d.Name()), s)
			}
			if !exists(srel) {
				brokenLinks = append(brokenLinks, BrokenLink{File: p, Link: s})
			}
		}
		return nil
	}
	if err := fs.WalkDir(fsys, ".", fn); err != nil {
		return err
	}
	if len(brokenLinks) != 0 {
		return &BrokenLinksError{Links: brokenLinks}
	}
	return nil
}

func extractLocalLinks(body []byte) ([]string, error) {
	var links []string
	fn := func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindAutoLink:
			if l, ok := n.(*ast.AutoLink); ok && l.AutoLinkType == ast.AutoLinkURL {
				links = append(links, string(l.URL(body)))
			}
		case ast.KindLink:
			if l, ok := n.(*ast.Link); ok {
				links = append(links, string(l.Destination))
			}
		case ast.KindImage:
			if l, ok := n.(*ast.Image); ok {
				links = append(links, string(l.Destination))
			}
		}
		return ast.WalkContinue, nil
	}
	node := mdparser.Parse(text.NewReader(body))
	if err := ast.Walk(node, fn); err != nil {
		return nil, err
	}
	out := links[:0]
	for _, s := range links {
		u, err := url.Parse(s)
		if err != nil || u.Scheme != "" || u.Host != "" {
			continue
		}
		out = append(out, u.Path)
	}
	return out[:len(out):len(out)], nil
}

type BrokenLinksError struct {
	Links []BrokenLink
}

func (e *BrokenLinksError) Error() string { return "broken links found" }

type BrokenLink struct {
	File string
	Link string
}

var mdparser = goldmark.DefaultParser()
