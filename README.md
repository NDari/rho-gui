# Rho GUI

`rho-gui` is a desktop frontend for [Rho](https://github.com/NDari/rho), a minimal coding agent. It uses the same configuration, sessions, skills, prompt templates and extensions as the `rho` command, so you can move between the two: a session started in the terminal can be resumed in the GUI and the other way around.

> **Status:** early. The GUI covers the main workflow (chat, tool cards, diffs, sessions and branching, settings, extensions) but has rough edges.

## Installing

Download the file for your platform from the [latest release](https://github.com/NDari/rho-gui/releases/latest):

| Platform | File |
|---|---|
| macOS (Apple silicon) | `Rho_<version>_macos.zip` |
| Linux (x86-64, ARM64) | `rho-gui_<version>_linux_<arch>.tar.gz` |
| Windows (x86-64) | `rho-gui_<version>_windows_amd64.zip` |

Each holds `rho-gui` and a `rho` built from the same version of Rho. The GUI uses that `rho` to run Go extensions, which starts them faster and with less memory than running them through `rho-gui`, so keep the two together. For Rho in the terminal, install it from the [Rho releases](https://github.com/NDari/rho#install).

- **macOS:** unzip the file and move `Rho.app` to Applications. Unless the release is signed and notarized, macOS blocks the first start; allow it under System Settings, Privacy & Security, or remove the quarantine flag with `xattr -dr com.apple.quarantine /Applications/Rho.app`.
- **Linux:** unpack the archive and put `rho-gui` and `rho` in the same folder, for example `~/.local/bin`. For an entry in the application menu with the Rho icon, install the files from the archive's `linux/` folder:

  ```sh
  mkdir -p ~/.local/share/applications ~/.local/share/icons/hicolor/scalable/apps
  cp linux/io.github.ndari.rho.desktop ~/.local/share/applications/
  cp linux/io.github.ndari.rho.svg ~/.local/share/icons/hicolor/scalable/apps/
  ```

  Wayland desktops take the window icon from this entry, so without it the window shows a generic icon.
- **Windows:** unzip the file and keep `rho-gui.exe` and `rho.exe` together.

### Requirements

The GUI draws its window with the system webview, which must be installed:

| Platform | Needs |
|---|---|
| Windows 10/11 | WebView2 (included with current Windows versions) |
| macOS | nothing extra |
| Linux | GTK 4.8+ and WebKitGTK 6.0 (2.40+), e.g. `libgtk-4-1` and `libwebkitgtk-6.0-4` on Debian and Ubuntu, `gtk4` and `webkitgtk-6.0` on Arch |

## Running

```sh
rho-gui                 # the current directory (or your home directory when started from a launcher)
rho-gui path/to/project
rho-gui -c              # continue the last session in the directory
rho-gui -m openai/<model-name>  # pick a model
rho-gui -session FILE   # open a specific session file
rho-gui -no-extensions
```

Configure models in `~/.rho/config.toml` first, as described in Rho's [quick start](https://github.com/NDari/rho#quick-start) and [configuration guide](https://github.com/NDari/rho/blob/main/docs/configuration.md). If Rho cannot start (for example because no model is configured yet), the window shows the error with an example configuration for a local llama.cpp server. When there is no config file yet, **Create the config file with this example** writes it and opens the project; otherwise fix the file and press **Try again**.

To work in another project, use `/open` (or **Open another folder…** in an empty conversation) to pick a folder. `/open path/to/project` skips the dialog. Switching starts a new session in that folder.

## Updating

Release builds look for a new version once a day. When there is one, a notice in the top right corner offers **Update and restart**: the GUI downloads the release, checks it against the release's checksums, replaces `rho-gui` and the `rho` next to it (or `Rho.app`) and opens again in the same folder and session. The button waits while the agent is working. **Later** hides the notice until the next start.

When the folder the GUI runs from is not writable, the update goes to `~/.local/bin` (`~/Applications` for `Rho.app`, `%LOCALAPPDATA%\Programs\Rho` on Windows) and the notice says what to do about the old copy and your `PATH`. On Linux, a desktop entry that starts `rho-gui` from the old folder keeps starting the old copy until you change it.

The check uses the same settings as `rho`: set `update_check = false` in `~/.rho/config.toml` or set `RHO_NO_UPDATE_CHECK` to turn it off. The GUI keeps its last result in `~/.rho/update-gui.json`.

## Using it

The window works like Rho's TUI:

- **Enter** sends, **Shift+Enter** or **Alt+Enter** adds a line.
- While the agent works, **Enter** steers it (interrupts after the current tool), **Alt+Enter** queues a follow-up and **Esc** (or the Stop button) cancels.
- Type `/` for commands: `/model`, `/new`, `/open`, `/resume`, `/rewind`, `/fork`, `/tree`, `/aside`, `/back`, `/compact`, `/context`, `/label`, `/copy`, `/theme`, `/config`, `/extensions`, `/reload`, `/help` and `/quit`, plus prompt templates and extension commands. **Tab** completes a command.
- `@path` attaches a file, as in the TUI; typing `@` completes file names. **Esc** hides a completion.
- **Ctrl+F** finds text in the conversation (**Enter** and **Shift+Enter** move between matches). Collapsed tool output is not searched; **Ctrl+O** expands it.
- Click a tool card's header to expand or collapse it. **Ctrl+O** expands all of them, **Ctrl+T** shows the model's thinking.
- `/theme` picks light, dark or the system setting. The GUI keeps this choice itself; the `theme` setting in the config file is for the TUI.
- Links in answers open in your browser.

## Building from source

You need Go 1.27 or newer and Node.js 22.12 or newer:

```sh
git clone https://github.com/NDari/rho-gui.git
cd rho-gui/frontend
npm ci
npm run build    # generates the Go bindings and builds the web assets into dist/
cd ..
CGO_ENABLED=0 go build -tags purego,production -o rho-gui .
CGO_ENABLED=0 go build -o rho github.com/NDari/rho/cmd/rho
```

The second build gives you the `rho` to keep next to `rho-gui`, from the Rho version in `go.mod`. A build from source has no version, so it does not look for updates.

With [Task](https://taskfile.dev) installed:

| Task | What it does |
|---|---|
| `task gui` | builds `bin/rho-gui` and `bin/rho` and opens the folder you ran `task` from |
| `task build` | builds both into `bin/` |
| `task install` | installs `rho-gui` into Go's `bin` folder |
| `task test` | checks the frontend and runs the Go tests |

Arguments after `--` go to the program, for example `task gui -- path/to/project`. The frontend is built again only when its files change.

The GUI is built with [Wails](https://wails.io) v3 and its pure-Go backend (the `purego` tag), so no C compiler is needed and you can cross-compile with `GOOS` and `GOARCH`. On Windows, add `-ldflags "-H windowsgui"` so no console window opens.

The icons are generated from `build/appicon.svg` with `sh build/icons.sh`. Release builds also run `build/windows/syso.sh`, which adds the icon, a high-DPI manifest and version information to the Windows program, and `build/darwin/bundle.sh` (on macOS), which builds `Rho.app`.

See [docs/design.md](docs/design.md) for how the GUI is put together, and how to work on it together with a local copy of Rho.

## Signing macOS releases

Releases are signed and notarized when these repository secrets are set; otherwise the macOS binaries and `Rho.app` are unsigned (ad-hoc signed).

| Secret | Holds |
|---|---|
| `MACOS_SIGN_P12` | a Developer ID Application certificate with its private key, exported as `.p12` and base64 encoded |
| `MACOS_SIGN_PASSWORD` | the `.p12` password |
| `MACOS_NOTARY_KEY` | an App Store Connect API key (`.p8`), base64 encoded |
| `MACOS_NOTARY_KEY_ID` | that key's ID |
| `MACOS_NOTARY_ISSUER_ID` | the issuer ID shown with the key |

GoReleaser signs and notarizes the `rho-gui` and `rho` binaries, and the `macos-app` job of the release workflow signs, notarizes and staples `Rho.app`.

## License

[MIT](LICENSE)
