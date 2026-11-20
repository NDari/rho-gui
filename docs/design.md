# Rho GUI design

A desktop frontend for [Rho](https://github.com/NDari/rho) built with [Wails](https://wails.io) v3. It is a frontend like Rho's TUI: it drives `core/app` and the agent directly and renders the `core/events` stream. No JSON-RPC is involved. Rho's [design spec](https://github.com/NDari/rho/blob/main/docs/rho-spec.md) covers the core it builds on.

## Repository and module

- The GUI is a **separate binary, `rho-gui`**, in its own repository and Go module (`github.com/NDari/rho-gui`), released on its own. Rho's module, `go build ./...` and the `rho` binary never depend on Wails or GTK.
- Keeping it out of `rho` matters: linking Wails loads GTK and WebKitGTK when the program starts, even if no window is opened, and needs those libraries on the machine. `rho` must stay a static binary that runs on servers without a desktop.
- `go.mod` pins the version of `github.com/NDari/rho` the GUI is built with. To work on both at once, create a `go.work` next to `go.mod` (it is ignored by git) that uses the local Rho checkout:

  ```
  go 1.27.4

  use (
  	.
  	../rho
  )
  ```

  A change to Rho's `core/` that the GUI needs is released (or at least pushed) in Rho first; then `go get github.com/NDari/rho@<version or commit>` here.
- `rho-gui` is built with Wails' experimental pure-Go backend (`CGO_ENABLED=0`, `-tags purego,production`), so every platform builds and cross-compiles without a C toolchain. It is not fully static: it loads the system webview at runtime (WebView2 on Windows, WKWebView on macOS, GTK 4.8+ and WebKitGTK 6.0 on Linux). If the pure-Go backend misbehaves, dropping the tag selects Wails' cgo backend with the same code.

## Structure

- **Go bridge** (`bridge/`, wired up in `main.go`): creates the App, forwards each event to the frontend as JSON (`events.Marshal`) over Wails events, exposes the command API (send, steer, follow-up, cancel, model, sessions, branching, compaction, settings, extensions) as bound service methods, and implements the extension UI (`Confirm`, `Select`, `Input`, `Notify`, `SetStatus`) as modal dialogs answered by the frontend.
- **Frontend** (`frontend/`): Svelte + TypeScript, built with Vite; the built assets are embedded in the binary. `npm run build` first generates the TypeScript bindings for the bridge with `go tool wails3`.
- **Shared view helpers** in Rho's `core/app` (transcript rebuilding, message lists for rewind and fork, the session tree, `@file` attachments and completion) keep session logic out of both frontends, so the TUI and GUI only differ in presentation.

## Behavior

- Same capabilities as the TUI: streaming markdown and thinking, tool cards with live output, diffs from the structured display data, status bar (model, context, cost, position), steering and follow-ups, the slash commands, prompt templates and extension commands.
- Keys follow the TUI where it makes sense: Enter sends, Shift+Enter adds a line, while working Enter steers and Alt+Enter queues a follow-up, Esc cancels.
- Light and dark themes; the GUI follows the system setting unless the user picks one.
- Ctrl+F searches the rendered transcript (what is on screen; collapsed tool output is not searched).
- The window can switch to another project directory (`/open`, a native folder dialog) with a new session, without restarting.
- When the project cannot be opened (no model configured, a broken config file), the window still opens and shows the error, an example configuration and a way to try again, since a GUI started from a desktop launcher has no terminal for the message. Without a config file it offers to create one from the example.
- Go extensions run as `<host> ext-host <file>`. `rho-gui` prefers a `rho` binary next to itself as the host, or one on `PATH` built from the same Rho version (compared with the `github.com/NDari/rho` version in its build information), because starting `rho-gui` again would load the GUI libraries in every extension process; it falls back to itself. The release archives and `Rho.app` include `rho` for this reason.
- Packaging: the icon comes from `build/appicon.svg`. The application ID `io.github.ndari.rho` names the Linux `.desktop` file (GTK 4 has no per-window icons, so Wayland desktops take the icon from it) and the macOS bundle. Releases add Windows resources (icon, high-DPI manifest, version information) and an Apple silicon `Rho.app` for macOS, signed and notarized when Apple credentials are configured.

## Releases and updates

### Release set

- One archive per platform with `rho-gui` and `rho`: `rho-gui_<version>_linux_amd64.tar.gz`, `rho-gui_<version>_linux_arm64.tar.gz` (both with the desktop entry and icon) and `rho-gui_<version>_windows_amd64.zip`.
- macOS (Apple silicon only): `Rho_<version>_macos.zip` holds `Rho.app` with both programs. The macOS archive goes only to the job that builds `Rho.app` and is not published.
- The bundled `rho` is built from the Rho version in `go.mod` (a `tool` dependency, so its packages are in `go.sum`) and reports that version, which the release workflow passes to goreleaser as `RHO_VERSION`.
- `checksums.txt` covers every published file. goreleaser creates the release as a draft; the `macos-app` job adds `Rho.app` and its checksum, then publishes it, so a published release is always complete.

### Updates

- Rho's `core/update` does the work, with a `Product` that names this repository, the `rho-gui` archives, both programs and `Rho.app`. The last check is kept in `~/.rho/update-gui.json`, separate from `rho`'s `update.json`.
- Checks run at most once a day and follow Rho's settings: `update_check = false` or `RHO_NO_UPDATE_CHECK` turn them off. Builds without a version never check.
- Release builds offer a newer version in a notice: **Update and restart** downloads this platform's archive (or `Rho_<version>_macos.zip` when running from `Rho.app`), checks it against `checksums.txt`, replaces `rho-gui` and the `rho` next to it (or `Rho.app` as a whole) and starts the new `rho-gui` with the same folder and session (`-session`, once the session has a file). It is disabled while the agent works. **Later** hides it until the next start.
- When the folder is not writable, the update goes to `~/.local/bin` (`~/Applications` for `Rho.app`, `%LOCALAPPDATA%\Programs\Rho` on Windows) without `sudo`, and the notice says to add that folder to `PATH`, or that the old copy comes first on `PATH`. The old copy is left alone.

## Next

- Speed up the CI job (about 7 minutes): cache the `wails3` tool build used for bindings, and check what else the job rebuilds each run.
- Test `rho-gui` on Windows (only cross-compiled so far), including the update and the absence of console windows.
- Releases stay unsigned for now. macOS signing and notarization start working once the secrets in the README are added; Windows signing is not set up.
