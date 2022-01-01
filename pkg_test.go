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
		{"index.md", LinkInfo{Raw: "three.md", Path: "three.md"}, kindFileNotExists},
		{"one.md", LinkInfo{Raw: "/three.md", Path: "/three.md"}, kindFileNotExists},
		{"subdir/two.md", LinkInfo{Raw: "../three.md#hi", Path: "../three.md", Fragment: "hi"}, kindFileNotExists},
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
