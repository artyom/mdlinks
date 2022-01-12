// Package mdlinks provides functions to verify cross document links in a set
// of markdown files.
package mdlinks

import (
	"bytes"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// Checker allows checks customization.
//
// Usage example:
//
// 	c := &mdlinks.Checker{
// 	    Matcher: func(s string) (bool, error) { return path.Ext(s) == ".md", nil },
// 	}
// 	err := c.CheckFS(os.DirFS(dir))
type Checker struct {
	// Matcher takes /-separated paths when CheckFS method traverses filesystem
	// (see documentation on fs.WalkDirFunc, its first argument). If Matcher
	// returns a non-nil error, CheckFS stops and returns this error. If
	// Matcher returns true, file is considered an utf-8 markdown document and
	// is processed.
	Matcher func(path string) (bool, error)
}

// CheckFS walks file system fsys looking for files using the Matcher function.
// It parses matched files as markdown, looks for local urls (urls that don't
// have schema and domain), and reports if it finds any urls pointing to
// non-existing files.
//
// If error returned is a *BrokenLinksError, it describes found files with
// broken links.
func (c *Checker) CheckFS(fsys fs.FS) error {
	if c == nil {
		panic("mdlinks: CheckFS called on a nil Checker")
	}
	if c.Matcher == nil {
		panic("mdlinks: CheckFS called with a nil Checker.Matcher")
	}
	exists := func(p string) bool {
		f, err := fsys.Open(p)
		if err != nil {
			return false
		}
		defer f.Close()
		return true
	}
	// track processed files to make sure each one is processed only once, even
	// if we need to get back to it at a later time to get its header ids. Keys
	// are full fsys paths.
	seen := make(map[string]*docDetails)
	getFileMeta := func(p string) (*docDetails, error) {
		docMeta, ok := seen[p]
		if ok {
			return docMeta, nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		if !utf8.Valid(b) {
			return nil, fmt.Errorf("%s is not a valid utf8 file", p)
		}
		if docMeta, err = extractDocDetails(b); err != nil {
			return nil, err
		}
		seen[p] = docMeta
		return docMeta, nil
	}
	var brokenLinks []BrokenLink
	fn := func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		switch ok, err := c.Matcher(p); {
		case err != nil:
			return err
		case !ok:
			return nil
		}
		docMeta, err := getFileMeta(p)
		if err != nil {
			return err
		}
		for _, s := range docMeta.links {
			var srel string // fs.FS relative path that link points to

			if s.Path != "" && s.Path[0] == '/' { // e.g. “/abc”
				srel = s.Path[1:]
			} else if s.Path != "" { // e.g. “abc” or “../abc”
				srel = path.Join(strings.TrimSuffix(p, d.Name()), s.Path)
			}
			// path is non-empty
			if srel != "" && !exists(srel) {
				brokenLinks = append(brokenLinks, BrokenLink{File: p, Link: s})
				continue
			}
			// path is empty, and fragment is non-empty (internal link)
			if s.Path == "" && s.Fragment != "" { // internal link
				if _, ok := docMeta.anchors[s.Fragment]; !ok {
					brokenLinks = append(brokenLinks, BrokenLink{File: p, Link: s, kind: kindBrokenInternalAnchor})
					continue
				}
			}
			if srel == "" || s.Fragment == "" {
				continue
			}
			if ok, _ := c.Matcher(srel); !ok {
				continue
			}
			// path is non-empty, fragment is non-empty, path points to the markdown file
			meta2, err := getFileMeta(srel)
			if err != nil {
				return err
			}
			if _, ok := meta2.anchors[s.Fragment]; !ok {
				brokenLinks = append(brokenLinks, BrokenLink{
					File: p,
					Link: s,
					kind: kindBrokenExternalAnchor,
				})
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

// CheckFS walks file system fsys looking for files with their base names
// matching pattern pat (e.g. “*.md”). It parses such files as markdown, looks
// for local urls (urls that don't have schema and domain), and reports if it
// finds any urls pointing to non-existing files.
//
// If error returned is a *BrokenLinksError, it describes found files with
// broken links.
func CheckFS(fsys fs.FS, pat string) error {
	if _, err := path.Match(pat, "xxx"); err != nil { // report bad pattern early
		return err
	}
	c := &Checker{
		Matcher: func(s string) (bool, error) { return path.Match(pat, path.Base(s)) },
	}
	return c.CheckFS(fsys)
}

type docDetails struct {
	links   []LinkInfo          // non-external links
	anchors map[string]struct{} // header slugs
}

func extractDocDetails(body []byte) (*docDetails, error) {
	// nodeContext returns numbers of the first and the last lines of the link
	// context: block element that contains it, usually paragraph
	nodeContext := func(n ast.Node) (int, int) {
		// only block type nodes have usable Lines() method, so if node is not
		// a block type, find its first block ancestor
		for n.Type() != ast.TypeBlock {
			if n.Type() == ast.TypeDocument {
				return 0, 0
			}
			if n = n.Parent(); n == nil {
				return 0, 0
			}
		}
		lines := n.Lines()
		if lines == nil || lines.Len() == 0 {
			return 0, 0
		}
		start := lines.At(0).Start
		stop := lines.At(lines.Len() - 1).Stop
		if stop == 0 || start == stop {
			return 0, 0
		}
		startLine := 1 + bytes.Count(body[:start], []byte{'\n'})
		endLine := startLine + bytes.Count(body[start:stop], []byte{'\n'})
		return startLine, endLine
	}

	var localLinks []LinkInfo
	var anchors map[string]struct{}

	// localLink parses s and returns *url.URL only if the link is local
	// (schema-less and domain-less link)
	localLink := func(s string) *url.URL {
		if s == "" {
			return nil
		}
		u, err := url.Parse(s)
		if err != nil || u.Scheme != "" || u.Host != "" {
			return nil
		}
		if u.Path == "" && u.Fragment == "" {
			return nil
		}
		return u
	}
	fn := func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var u *url.URL
		var raw string // link target as seen in the document body
		switch n.Kind() {
		case ast.KindHeading:
			if n, ok := n.(*ast.Heading); ok {
				if text := nodeText(n, body); text != "" {
					name := slugify(text)
					if anchors == nil {
						anchors = make(map[string]struct{})
					}
					for i := 0; i < 100; i++ {
						var cand string
						if i == 0 {
							cand = name
						} else {
							cand = fmt.Sprintf("%s-%d", name, i)
						}
						if _, ok := anchors[cand]; !ok {
							anchors[cand] = struct{}{}
							break
						}
					}
				}
			}
		case ast.KindAutoLink:
			if l, ok := n.(*ast.AutoLink); ok && l.AutoLinkType == ast.AutoLinkURL {
				raw = string(l.URL(body))
				u = localLink(raw)
			}
		case ast.KindLink:
			if l, ok := n.(*ast.Link); ok {
				raw = string(l.Destination)
				u = localLink(raw)
			}
		case ast.KindImage:
			if l, ok := n.(*ast.Image); ok {
				raw = string(l.Destination)
				u = localLink(raw)
			}
		}
		if u != nil && raw != "" {
			l1, l2 := nodeContext(n)
			localLinks = append(localLinks, LinkInfo{
				Raw:       raw,
				Path:      u.Path,
				Fragment:  u.Fragment,
				LineStart: l1,
				LineEnd:   l2,
			})
		}
		return ast.WalkContinue, nil
	}
	node := mdparser.Parse(text.NewReader(body))
	if err := ast.Walk(node, fn); err != nil {
		return nil, err
	}
	return &docDetails{anchors: anchors, links: localLinks}, nil
}

// BrokenLinksError is an error type returned by this package functions to
// report found broken links.
//
// Usage example:
//
// 	err := mdlinks.CheckFS(os.DirFS(dir), "*.md")
// 	var e *mdlinks.BrokenLinksError
// 	if errors.As(err, &e) {
// 	    for _, link := range e.Links {
// 	        log.Println(link)
// 	    }
// 	}
type BrokenLinksError struct {
	Links []BrokenLink
}

func (e *BrokenLinksError) Error() string { return "broken links found" }

// BrokenLink describes broken markdown link and the file it belongs to.
type BrokenLink struct {
	File string // file path, relative to directory/filesystem scanned; uses '/' as a separator
	Link LinkInfo
	kind violationKind
}

func (b BrokenLink) String() string {
	switch b.kind {
	case kindBrokenInternalAnchor:
		return fmt.Sprintf("%s: link %q points to a non-existing local slug", b.File, b.Link.Raw)
	case kindBrokenExternalAnchor:
		return fmt.Sprintf("%s: link %q points to a non-existing slug", b.File, b.Link.Raw)
	}
	return fmt.Sprintf("%s: link %q points to a non-existing file", b.File, b.Link.Raw)
}

func (b BrokenLink) Reason() string { return b.kind.String() }

type violationKind byte

const (
	kindFileNotExists = iota
	kindBrokenInternalAnchor
	kindBrokenExternalAnchor
)

func (v violationKind) String() string {
	switch v {
	case kindBrokenInternalAnchor:
		return "link points to a non-existing local slug"
	case kindBrokenExternalAnchor:
		return "link points to a non-existing slug"
	}
	return "link points to a non-existing file"
}

// LinkInfo describes markdown link
type LinkInfo struct {
	Raw       string // as seen in the source, usually “some/path#fragment”
	Path      string // only the path part of the link
	Fragment  string // only the fragment part of the link, without '#'
	LineStart int    // number of the first line of the context (usually paragraph)
	LineEnd   int    // number of the last line of the context (usually paragraph)
}

var mdparser = parser.NewParser(
	parser.WithBlockParsers(parser.DefaultBlockParsers()...),
	parser.WithInlineParsers(parser.DefaultInlineParsers()...),
	parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
)

// nodeText walks node and extracts plain text from it and its descendants,
// effectively removing all markdown syntax
func nodeText(node ast.Node, src []byte) string {
	var b strings.Builder
	fn := func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.Kind() {
		case ast.KindText:
			if t, ok := n.(*ast.Text); ok {
				b.Write(t.Text(src))
			}
		}
		return ast.WalkContinue, nil
	}
	if err := ast.Walk(node, fn); err != nil {
		return ""
	}
	return b.String()
}

func slugify(text string) string {
	f := func(r rune) rune {
		switch {
		case r == '-' || r == '_':
			return r
		case unicode.IsSpace(r):
			return '-'
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			return unicode.ToLower(r)
		}
		return -1
	}
	return strings.Map(f, text)
}
