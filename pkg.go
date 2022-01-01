// Package mdlinks provides functions to verify cross links in a set of
// markdown files.
package mdlinks

import (
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
		if d.IsDir() {
			return nil
		}
		if ok, _ := path.Match(pat, d.Name()); !ok {
			return nil
		}
		docMeta, err := getFileMeta(p)
		if err != nil {
			return err
		}
		// log.Printf("DEBUG: %s: %v", p, docMeta.anchors)
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
			if ok, _ := path.Match(pat, path.Base(srel)); !ok {
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

type docDetails struct {
	links   []LinkInfo          // non-external links
	anchors map[string]struct{} // header slugs
}

func extractDocDetails(body []byte) (*docDetails, error) {
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
	idgen := new(idGenerator)
	node := mdparser.Parse(text.NewReader(body), parser.WithContext(parser.NewContext(parser.WithIDs(idgen))))
	if err := ast.Walk(node, fn); err != nil {
		return nil, err
	}
	local := make([]LinkInfo, 0)
	for _, s := range links {
		u, err := url.Parse(s)
		if err != nil || u.Scheme != "" || u.Host != "" {
			continue
		}
		if u.Path == "" && u.Fragment == "" {
			continue
		}
		local = append(local, LinkInfo{Raw: s, Path: u.Path, Fragment: u.Fragment})
	}
	out := &docDetails{anchors: idgen.seen}
	if l := len(local); l != 0 {
		out.links = make([]LinkInfo, l)
		copy(out.links, local)
	}
	return out, nil
}

// BrokenLinksError is an error type returned by this package functions to
// report found broken links.
//
// Usage example:
//
// 	err := mdlinks.CheckFS(os.DirFS(dir), "*.md")
// 	var e *mdlinks.BrokenLinksError
// 	if errors.As(err, &e) {
// 		for _, link := range e.Links {
// 			log.Println(link)
// 		}
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

func (b BrokenLink) Reason() string {
	switch b.kind {
	case kindBrokenInternalAnchor:
		return "link points to a non-existing local slug"
	case kindBrokenExternalAnchor:
		return "link points to a non-existing slug"
	}
	return "link points to a non-existing file"
}

type violationKind byte

const (
	kindFileNotExists = iota
	kindBrokenInternalAnchor
	kindBrokenExternalAnchor
)

// LinkInfo describes markdown link
type LinkInfo struct {
	Raw      string // as seen in the source, usually “some/path#fragment”
	Path     string // only the path part of the link
	Fragment string // only the fragment part of the link, without '#'
}

var mdparser = parser.NewParser(
	parser.WithBlockParsers(parser.DefaultBlockParsers()...),
	parser.WithInlineParsers(parser.DefaultInlineParsers()...),
	parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	parser.WithAutoHeadingID(),
)

// idGenerator creates ids for HTML headers
type idGenerator struct {
	seen map[string]struct{}
}

func (g *idGenerator) Generate(value []byte, kind ast.NodeKind) []byte {
	if kind != ast.KindHeading || len(value) == 0 {
		return nil
	}
	if g.seen == nil {
		g.seen = make(map[string]struct{})
	}
	var anchorName []rune
	var futureDash = false
	for _, r := range string(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			if futureDash && len(anchorName) > 0 {
				anchorName = append(anchorName, '-')
			}
			futureDash = false
			anchorName = append(anchorName, unicode.ToLower(r))
		default:
			futureDash = true
		}
	}
	name := string(anchorName)
	for i := 0; i < 100; i++ {
		var cand string
		if i == 0 {
			cand = name
		} else {
			cand = fmt.Sprintf("%s-%d", name, i)
		}
		if _, ok := g.seen[cand]; !ok {
			g.seen[cand] = struct{}{}
			return []byte(cand)
		}
	}
	return nil
}

func (g *idGenerator) Put(value []byte) {}
