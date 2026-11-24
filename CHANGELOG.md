# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Bump go-gui v0.99.1 to v0.100.0, go-glyph v1.29.2 to v1.29.3, go-charts
  v0.34.0 to v0.35.0 and go-map v0.32.0 to v0.33.0.** Dependency update.

## [v0.2.2] - 2026-11-19

### Changed

- Dependency bumps only, no app changes (#22-#34): go-gui v0.84.0 → v0.98.0,
  go-charts v0.22.0 → v0.33.0, go-map v0.20.0 → v0.31.0, go-glyph v1.26.1 →
  v1.28.0 (plus indirect bumps).
- Pinned golangci-lint in a tools/lint module; CI runs `make lint` (#22).
