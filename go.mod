module github.com/artyom/mdlinks

go 1.18

require github.com/yuin/goldmark v1.4.4

retract [v0.3.0, v0.3.1] // Incorrectly handles _ and - when generating header ids.
