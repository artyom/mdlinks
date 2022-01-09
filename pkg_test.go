package mdlinks

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_slugify(t *testing.T) {
	testCases := []struct {
		text, want string
	}{
		{`Foo/Bar`, `foobar`},
		{`Foo (Bar)`, `foo-bar`},
		{`-Client-Side`, `-client-side`},
		{`A [Link](https://example.org/) Inside`, `a-link-inside`},
		{`Header *with formatting*`, `header-with-formatting`},
		{`Header with & symbol`, `header-with--symbol`},
		{`Punctuation,   and    repeating:  spaces`, `punctuation---and----repeating--spaces`},
	}
	for _, c := range testCases {
		got := slugify([]byte(c.text))
		if got != c.want {
			t.Errorf("text: %q, got %q, want %q", c.text, got, c.want)
		}
	}
}

func TestCheckFS(t *testing.T) {
	err := CheckFS(os.DirFS(filepath.FromSlash("testdata/a")), "*.md")
	var e *BrokenLinksError
	if !errors.As(err, &e) {
		t.Fatalf("want *ErrBrokenLinks, got %v", err)
	}
	want := strings.Join([]string{
		`index.md: link "three.md" points to a non-existing file`,
		`one.md: link "/three.md" points to a non-existing file`,
		`subdir/two.md: link "../three.md#hi" points to a non-existing file`,
	}, "\n")
	b := new(strings.Builder)
	for i, link := range e.Links {
		if i != 0 {
			b.WriteString("\n")
		}
		b.WriteString(link.String())
	}
	if got := b.String(); got != want {
		t.Fatalf("got:\n%s\n\nwant:\n%s", got, want)
	}
	gotLink := e.Links[2]
	wantLink := BrokenLink{
		File: "subdir/two.md",
		Link: LinkInfo{
			Raw:       "../three.md#hi",
			Path:      "../three.md",
			Fragment:  "hi",
			LineStart: 3,
			LineEnd:   4,
		},
	}

	if gotLink != wantLink {
		t.Fatalf("got link %#v, want %#v", gotLink, wantLink)
	}

	// create missing file
	dir := t.TempDir()
	if err := copyDirectory(dir, filepath.FromSlash("testdata/a")); err != nil {
		t.Fatalf("copying testdata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "three.md"), []byte("## Hi!\n"), 0666); err != nil {
		t.Fatal(err)
	}
	if err := CheckFS(os.DirFS(dir), "*.md"); err != nil {
		var e *BrokenLinksError
		if errors.As(err, &e) {
			for _, link := range e.Links {
				t.Logf("%+v", link)
			}
		}
		t.Fatal(err)
	}
}

func copyDirectory(dst, src string) error {
	srcFS := os.DirFS(src)
	fn := func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0777)
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		f, err := srcFS.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := io.Copy(out, f); err != nil {
			return err
		}
		return out.Close()
	}
	return fs.WalkDir(srcFS, ".", fn)
}
