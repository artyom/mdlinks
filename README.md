# mdlinks

This repository provides [Go package](https://pkg.go.dev/github.com/artyom/mdlinks), [command-line tool](#command-line-tool), and [a GitHub Action](#github-action)
that can verify cross-document links in a collection of markdown files.

Code scans markdown files for domain-less links and checks if referenced files exist.
For example, if a file “doc1.md” has a link with “../img.png” target,
this tool will check whether the “img.png” file exists in a “doc1.md” file parent directory.

If a link references an existing markdown document and has a fragment part,
this tool checks that such a link points to an existing markdown header,
following similar (but not exactly matching) rules of unique ID generating as GitHub markdown rendering.

For example, use the `#table-of-contents` link fragment to reference the “Table of Contents” header.

Using special symbols or extra formatting in the header will likely produce an ID that differs from what GitHub could have generated.

## Command-line tool

Install it like:

```sh
go install github.com/artyom/mdlinks/cmd/mdlinks@latest
```

## GitHub Action

When using default settings (scan the repository root directory, look for `*.md` files),
you can use this action like this:

```yaml
on:
  push:
  pull_request:

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      - uses: artyom/mdlinks@main
```

You can customize what directory to scan and which files to match:

```yaml
- uses: artyom/mdlinks@main
  with:
    dir: 'docs'
    glob: '*.markdown'
```
