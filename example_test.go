package mdlinks_test

import (
	"errors"
	"fmt"
	"testing/fstest"

	"github.com/artyom/mdlinks"
)

const Doc1 = `
# Document One

Text with a [broken link](non-existing-file.md).
`

const Doc2 = `
# Document Two

Example of [internal](#document-two),
and [external](../doc1.md#document-one) links.

No such [reference][1].

[1]: #invalid-ref
`

func Example() {
	fs := make(fstest.MapFS) // in real scenario this will likely be os.DirFS(dir)
	writeFile(fs, "doc1.md", Doc1)
	writeFile(fs, "subdir/doc2.md", Doc2)
	err := mdlinks.CheckFS(fs, "*.md")
	var e *mdlinks.BrokenLinksError
	if errors.As(err, &e) {
		for _, link := range e.Links {
			fmt.Println(link)
		}
	}
	// Output:
	// doc1.md: link "non-existing-file.md" points to a non-existing file
	// subdir/doc2.md: link "#invalid-ref" points to a non-existing local slug
}

func writeFile(fs fstest.MapFS, name, body string) {
	fs[name] = &fstest.MapFile{Data: []byte(body)}
}
