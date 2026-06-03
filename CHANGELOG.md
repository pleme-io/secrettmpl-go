# Changelog

All notable changes to this project are documented here.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-03

### Added
- Initial: `Render(template, resolver)` over the `<path:…>` angle-bracket
  placeholder syntax with inline (`<ignore:path:…>`) and block
  (`<ignore>…</ignore>`) ignore annotations; `Resolver` interface +
  `ResolverFunc` + `MapResolver` adapters; single-pass backtrack-free scanner;
  typed code-carrying errors via `errors-go` (`secrettmpl_unterminated` /
  `secrettmpl_empty_path` / `secrettmpl_unresolved` / `secrettmpl_resolve` /
  `secrettmpl_nil_resolver`).
