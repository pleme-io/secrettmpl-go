// Package secrettmpl is the fleet's one path-placeholder templating engine —
// the angle-bracket <path:…> syntax with ignore-annotation semantics — so no
// tool hand-rolls its own placeholder scanner over secret-injection templates.
//
// A template is arbitrary text with embedded placeholders of the form
//
//	<path:some/secret/path>
//
// [Render] walks the template left-to-right and replaces each placeholder with
// the value returned by a [Resolver] for that path; all other text is copied
// verbatim. Two ignore-annotation forms suppress resolution so a literal
// "<path:…>" can survive into the output (for documentation, nested templates,
// or escaping):
//
//   - inline:  <ignore:path:some/path>  →  renders the literal "<path:some/path>"
//   - block:   <ignore> … </ignore>     →  every placeholder inside is left
//     untouched (emitted verbatim, ignore tags stripped)
//
// The scanner is a single pass with no backtracking; unterminated or malformed
// placeholders yield typed, code-carrying errors via errors-go. Pure stdlib +
// errors-go.
package secrettmpl

import (
	"strings"

	errs "github.com/pleme-io/errors-go"
)

const (
	openPath   = "<path:"
	openIgnore = "<ignore:"
	blockOpen  = "<ignore>"
	blockClose = "</ignore>"
	closeTag   = ">"
)

// Resolver maps a placeholder path to its value. Implementations resolve a
// path (e.g. a secret store key) to the string substituted into the output.
// Returning an error aborts the whole render with that error wrapped in a
// typed "secrettmpl_resolve" error identifying the offending path.
type Resolver interface {
	Resolve(path string) (string, error)
}

// ResolverFunc adapts a plain function to the [Resolver] interface.
type ResolverFunc func(path string) (string, error)

// Resolve calls the underlying function.
func (f ResolverFunc) Resolve(path string) (string, error) { return f(path) }

// MapResolver is a [Resolver] backed by a static map, the convenience form for
// fixed substitution sets and tests. A path absent from the map resolves to an
// "secrettmpl_unresolved" error.
type MapResolver map[string]string

// Resolve looks up path in the map.
func (m MapResolver) Resolve(path string) (string, error) {
	v, ok := m[path]
	if !ok {
		return "", errs.New("secrettmpl: no value for path "+path,
			errs.WithCode("secrettmpl_unresolved"))
	}
	return v, nil
}

// Render expands every <path:…> placeholder in template using resolver,
// honoring inline <ignore:path:…> and block <ignore>…</ignore> annotations.
// It returns the fully substituted string, or a typed code-carrying error on a
// malformed placeholder ("secrettmpl_unterminated" / "secrettmpl_empty_path")
// or a resolver failure ("secrettmpl_resolve").
func Render(template string, resolver Resolver) (string, error) {
	if resolver == nil {
		return "", errs.New("secrettmpl: nil resolver",
			errs.WithCode("secrettmpl_nil_resolver"))
	}

	var b strings.Builder
	b.Grow(len(template))

	rest := template
	for len(rest) > 0 {
		// Find the next interesting token: a block-ignore open, an inline
		// ignore, or a path placeholder — whichever comes first.
		nextBlock := strings.Index(rest, blockOpen)
		nextIgnore := strings.Index(rest, openIgnore)
		nextPath := strings.Index(rest, openPath)

		idx, kind := earliest(nextBlock, nextIgnore, nextPath)
		if idx < 0 {
			// No more tokens; emit the remainder verbatim.
			b.WriteString(rest)
			break
		}

		// Emit everything before the token verbatim.
		b.WriteString(rest[:idx])
		rest = rest[idx:]

		switch kind {
		case tokenBlock:
			// <ignore> … </ignore>: copy the inner text verbatim, dropping the
			// tags; placeholders inside are NOT resolved.
			inner, after, err := cutBlock(rest)
			if err != nil {
				return "", err
			}
			b.WriteString(inner)
			rest = after

		case tokenIgnore:
			// <ignore:path:foo>: emit the literal "<path:foo>".
			body, after, err := cutTag(rest, openIgnore)
			if err != nil {
				return "", err
			}
			lit, err := literalFromIgnore(body)
			if err != nil {
				return "", err
			}
			b.WriteString(lit)
			rest = after

		case tokenPath:
			// <path:foo>: resolve and substitute.
			path, after, err := cutTag(rest, openPath)
			if err != nil {
				return "", err
			}
			if path == "" {
				return "", errs.New("secrettmpl: empty path in <path:>",
					errs.WithCode("secrettmpl_empty_path"))
			}
			val, err := resolver.Resolve(path)
			if err != nil {
				return "", errs.Wrap(err, "secrettmpl: resolving path "+path,
					errs.WithCode("secrettmpl_resolve"))
			}
			b.WriteString(val)
			rest = after
		}
	}
	return b.String(), nil
}

// token kinds in scan order of precedence (earliest position wins).
type tokenKind int

const (
	tokenNone tokenKind = iota
	tokenBlock
	tokenIgnore
	tokenPath
)

// earliest returns the smallest non-negative index among the three candidates
// and the token kind it represents. When several share the same index, the
// more specific (longer-prefix) token wins: "<ignore>" and "<ignore:" both
// start at the same byte as nothing else, but "<ignore:path:…>" must not be
// mistaken for a block open — so a block open only wins if its index is
// strictly less than the inline-ignore index, otherwise inline wins the tie.
func earliest(block, ignore, path int) (int, tokenKind) {
	best := -1
	kind := tokenNone
	consider := func(i int, k tokenKind) {
		if i < 0 {
			return
		}
		if best < 0 || i < best {
			best, kind = i, k
		}
	}
	// Order of consideration encodes tie-breaking: inline ignore is considered
	// before block so that at an equal index "<ignore:" (inline) wins over
	// "<ignore>" (block). They can only share an index when the text is
	// "<ignore" followed by ":" vs ">", which are mutually exclusive next
	// bytes, so in practice only one matches at a given position anyway.
	consider(ignore, tokenIgnore)
	consider(block, tokenBlock)
	consider(path, tokenPath)
	return best, kind
}

// cutTag consumes a tag of the form "<open>BODY>" at the head of s (s must
// start with open) and returns BODY, the remainder after the closing ">", or a
// typed unterminated error.
func cutTag(s, open string) (body, after string, err error) {
	inner := s[len(open):]
	end := strings.Index(inner, closeTag)
	if end < 0 {
		return "", "", errs.New("secrettmpl: unterminated "+open+" placeholder",
			errs.WithCode("secrettmpl_unterminated"),
			errs.WithHint("close it with '>'"))
	}
	return inner[:end], inner[end+len(closeTag):], nil
}

// cutBlock consumes "<ignore> INNER </ignore>" at the head of s (s must start
// with blockOpen) and returns INNER verbatim, the remainder after </ignore>,
// or a typed unterminated error.
func cutBlock(s string) (inner, after string, err error) {
	body := s[len(blockOpen):]
	end := strings.Index(body, blockClose)
	if end < 0 {
		return "", "", errs.New("secrettmpl: unterminated <ignore> block",
			errs.WithCode("secrettmpl_unterminated"),
			errs.WithHint("close it with </ignore>"))
	}
	return body[:end], body[end+len(blockClose):], nil
}

// literalFromIgnore turns the body of an "<ignore:…>" tag back into the literal
// placeholder it shields. The supported form is "path:FOO", which renders as
// "<path:FOO>". Any other body is treated literally as "<BODY>" so the
// annotation is reversible for arbitrary tags.
func literalFromIgnore(body string) (string, error) {
	if body == "" {
		return "", errs.New("secrettmpl: empty <ignore:> annotation",
			errs.WithCode("secrettmpl_empty_path"))
	}
	return "<" + body + ">", nil
}
