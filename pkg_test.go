package mdurlcheck

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckFS(t *testing.T) {
	err := CheckFS(os.DirFS("testdata"), "*.md")
	var e *BrokenLinksError
	if !errors.As(err, &e) {
		t.Fatalf("want *ErrBrokenLinks, got %v", err)
	}
	expected := []BrokenLink{
		{"index.md", "three.md"},
		{"one.md", "/three.md"},
		{"subdir/two.md", "../three.md"},
	}
	if len(e.Links) != len(expected) {
		t.Fatalf("broken links got: %+v\nwant: %+v", e.Links, expected)
	}
	for i, link := range e.Links {
		if expected[i] != link {
			t.Fatalf("broken links got: %+v\nwant: %+v", e.Links, expected)
		}
	}
	// create missing file
	dir := t.TempDir()
	if err := copyDirectory(dir, "testdata"); err != nil {
		t.Fatalf("copying testdata: %v", err)
	}
	f, err := os.Create(filepath.Join(dir, "three.md"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
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
