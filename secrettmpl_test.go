package secrettmpl

import (
	stderrors "errors"
	"testing"

	errs "github.com/pleme-io/errors-go"
)

func TestRender(t *testing.T) {
	resolver := MapResolver{
		"db/password": "hunter2",
		"api/token":   "t-abc",
		"empty":       "",
	}

	tests := []struct {
		name string
		tmpl string
		want string
	}{
		{
			name: "no placeholders",
			tmpl: "plain text only",
			want: "plain text only",
		},
		{
			name: "single placeholder",
			tmpl: "pw=<path:db/password>",
			want: "pw=hunter2",
		},
		{
			name: "multiple placeholders",
			tmpl: "<path:api/token>:<path:db/password>",
			want: "t-abc:hunter2",
		},
		{
			name: "placeholder with surrounding text",
			tmpl: "Authorization: Bearer <path:api/token> end",
			want: "Authorization: Bearer t-abc end",
		},
		{
			name: "empty resolved value",
			tmpl: "x=<path:empty>=y",
			want: "x==y",
		},
		{
			name: "inline ignore renders literal placeholder",
			tmpl: "use <ignore:path:db/password> in your config",
			want: "use <path:db/password> in your config",
		},
		{
			name: "inline ignore alongside live placeholder",
			tmpl: "literal <ignore:path:api/token> live <path:api/token>",
			want: "literal <path:api/token> live t-abc",
		},
		{
			name: "block ignore passes inner through verbatim",
			tmpl: "before <ignore>raw <path:db/password> text</ignore> after",
			want: "before raw <path:db/password> text after",
		},
		{
			name: "block ignore then live placeholder",
			tmpl: "<ignore><path:db/password></ignore> then <path:api/token>",
			want: "<path:db/password> then t-abc",
		},
		{
			name: "adjacent placeholders",
			tmpl: "<path:api/token><path:db/password>",
			want: "t-abchunter2",
		},
		{
			name: "lone angle brackets are literal",
			tmpl: "a < b > c",
			want: "a < b > c",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Render(tc.tmpl, resolver)
			if err != nil {
				t.Fatalf("Render(%q): %v", tc.tmpl, err)
			}
			if got != tc.want {
				t.Fatalf("Render(%q) = %q, want %q", tc.tmpl, got, tc.want)
			}
		})
	}
}

func TestRenderErrors(t *testing.T) {
	resolver := MapResolver{"known": "v"}

	tests := []struct {
		name     string
		tmpl     string
		wantCode string
	}{
		{
			name:     "unterminated path",
			tmpl:     "x=<path:db/password",
			wantCode: "secrettmpl_unterminated",
		},
		{
			name:     "unterminated block",
			tmpl:     "<ignore>never closed",
			wantCode: "secrettmpl_unterminated",
		},
		{
			name:     "empty path",
			tmpl:     "x=<path:>",
			wantCode: "secrettmpl_empty_path",
		},
		{
			// MapResolver returns secrettmpl_unresolved, which Render wraps in
			// secrettmpl_resolve; CodeOf reports the outermost code (the
			// wrapper). The inner code is preserved in the chain (asserted in
			// TestUnresolvedInnerCodePreserved).
			name:     "unresolved path",
			tmpl:     "x=<path:missing>",
			wantCode: "secrettmpl_resolve",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Render(tc.tmpl, resolver)
			if err == nil {
				t.Fatalf("Render(%q): want error", tc.tmpl)
			}
			if errs.CodeOf(err) != tc.wantCode {
				t.Fatalf("Render(%q) code = %q, want %q", tc.tmpl, errs.CodeOf(err), tc.wantCode)
			}
		})
	}
}

func TestUnresolvedInnerCodePreserved(t *testing.T) {
	_, err := Render("x=<path:missing>", MapResolver{"known": "v"})
	if err == nil {
		t.Fatal("want error")
	}
	// Outermost code is the wrapper; the inner MapResolver code is preserved
	// in the cause chain.
	if errs.CodeOf(err) != "secrettmpl_resolve" {
		t.Fatalf("outer code = %q, want secrettmpl_resolve", errs.CodeOf(err))
	}
	inner := stderrors.Unwrap(err)
	if inner == nil {
		t.Fatal("want a wrapped cause")
	}
	if errs.CodeOf(inner) != "secrettmpl_unresolved" {
		t.Fatalf("inner code = %q, want secrettmpl_unresolved", errs.CodeOf(inner))
	}
}

func TestRenderNilResolver(t *testing.T) {
	_, err := Render("x=<path:a>", nil)
	if err == nil {
		t.Fatal("want error for nil resolver")
	}
	if errs.CodeOf(err) != "secrettmpl_nil_resolver" {
		t.Fatalf("code = %q, want secrettmpl_nil_resolver", errs.CodeOf(err))
	}
}

func TestResolverFunc(t *testing.T) {
	calls := 0
	r := ResolverFunc(func(path string) (string, error) {
		calls++
		return "resolved:" + path, nil
	})
	got, err := Render("<path:foo>/<path:bar>", r)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "resolved:foo/resolved:bar" {
		t.Fatalf("got %q", got)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestResolveErrorWrapsPath(t *testing.T) {
	r := ResolverFunc(func(path string) (string, error) {
		return "", errs.New("backend down", errs.WithCode("backend_err"))
	})
	_, err := Render("<path:secret/x>", r)
	if err == nil {
		t.Fatal("want error")
	}
	// Outermost code is the secrettmpl_resolve wrapper.
	if errs.CodeOf(err) != "secrettmpl_resolve" {
		t.Fatalf("code = %q, want secrettmpl_resolve", errs.CodeOf(err))
	}
	// The underlying backend error is preserved in the chain.
	if !contains(err.Error(), "backend down") {
		t.Fatalf("error %q does not mention cause", err.Error())
	}
	if !contains(err.Error(), "secret/x") {
		t.Fatalf("error %q does not mention path", err.Error())
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
