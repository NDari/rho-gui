# AGENTS.md

Guidance for coding agents and contributors working on Rho GUI.

## Project

Rho GUI (`rho-gui`) is a desktop frontend for [Rho](https://github.com/NDari/rho), built with Wails v3 and a Svelte + TypeScript frontend. [docs/design.md](docs/design.md) describes how it is put together; Rho's [design spec](https://github.com/NDari/rho/blob/main/docs/rho-spec.md) covers the core it uses. Update the design doc when a decision changes.

## Architecture rules

- The GUI is a frontend: it drives Rho's `core/app` and renders the `core/events` stream. Session, agent and config logic belongs in Rho's `core/`, not here.
- Changes to `core/` are made in the Rho repository first. Use a `go.work` with a local Rho checkout while working on both (see the design doc), then pin the new Rho version in `go.mod`.
- Bridge methods on `*bridge.Bridge` are bound to the frontend by the Wails bindings generator. Helpers the frontend must not call are package functions, not methods.

## Code conventions

- Standard Go style: `gofmt`, `go vet`, idiomatic error handling (wrap with `%w`, no panics).
- Build with `CGO_ENABLED=0` and `-tags purego` (add `production` for a real build). The program must keep cross-compiling for every release platform without a C toolchain.
- Platform-specific code goes in `_windows.go` / `_other.go` (or `_darwin.go`) files, not runtime `GOOS` checks scattered through logic.

## Testing

- `task test` (frontend check plus `go test -tags purego ./...`) and `go vet -tags purego ./...` must pass before a change is considered done.
- Tests must not require network access or API keys.

## Writing style

- Plain hyphens only; no em-dashes or en-dashes in code, comments or docs.
- Examples in docs must use generic values (e.g. `http://localhost:9931/v1`), never real hosts, usernames or personal paths.
- Commit messages: short imperative subject line, optional body explaining why.
