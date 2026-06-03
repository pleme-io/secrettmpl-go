module github.com/pleme-io/secrettmpl-go

go 1.25

require github.com/pleme-io/errors-go v0.1.0

// TEMP: cross-import a committed sibling locally until errors-go is published.
replace github.com/pleme-io/errors-go => ../errors-go
