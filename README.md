# secrettmpl-go

The fleet's one path-placeholder templating engine — the angle-bracket
`<path:…>` syntax with ignore-annotation semantics — so no tool hand-rolls its
own placeholder scanner over secret-injection templates.

## What

A template is arbitrary text with embedded placeholders:

```
<path:some/secret/path>
```

`Render(template, resolver)` walks the template left-to-right, replaces each
placeholder with the value the `Resolver` returns for that path, and copies all
other text verbatim. Two ignore-annotation forms suppress resolution so a
literal `<path:…>` survives into the output:

- inline: `<ignore:path:some/path>` renders the literal `<path:some/path>`
- block: `<ignore> … </ignore>` emits its inner text verbatim (placeholders
  inside are left untouched; the ignore tags are stripped)

The scanner is a single, backtrack-free pass. Unterminated or malformed
placeholders, an empty path, an unresolved path, and resolver failures each
become a typed, code-carrying error via `errors-go`
(`secrettmpl_unterminated` / `secrettmpl_empty_path` / `secrettmpl_unresolved`
/ `secrettmpl_resolve` / `secrettmpl_nil_resolver`).

## Why

Path-placeholder substitution over secret-injection templates recurs across
config rendering, env-file generation, and bundle producers. One typed engine
means uniform placeholder syntax, uniform ignore/escape semantics, and one
place that owns the scanner — never a hand-rolled `strings.Replace` loop that
mishandles escaping or unterminated tags again.

## Install

```
go get github.com/pleme-io/secrettmpl-go
```

## Usage

```go
out, err := secrettmpl.Render(
    "Authorization: Bearer <path:api/token>\n# docs: <ignore:path:api/token>",
    secrettmpl.ResolverFunc(func(p string) (string, error) {
        return store.Get(ctx, p) // any secret backend
    }),
)
if err != nil { return errs.Exit(err) }
// out: "Authorization: Bearer t-abc\n# docs: <path:api/token>"

// Static substitution (tests, fixed sets):
out, _ = secrettmpl.Render("pw=<path:db/pw>", secrettmpl.MapResolver{"db/pw": "x"})
```

## Configuration

None — a pure library. The placeholder paths come from the template; the
backend behind the `Resolver` is the caller's choice (a secret store, a config
map, …) and is wired with `shikumi-go` upstream as needed.

## Release

Pull-model (Go modules): an annotated `vX.Y.Z` tag is the release; pkg.go.dev
indexes it. See the GSDS module delivery FSM.
