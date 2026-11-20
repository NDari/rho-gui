// Command rho-gui is Rho's desktop frontend.
package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/NDari/rho/core/app"
	"github.com/NDari/rho/core/config"
	"github.com/NDari/rho/core/ext/yaegihost"
	"github.com/NDari/rho/core/update"
	"github.com/NDari/rho/gui/bridge"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// appID identifies the application to the desktop. On Linux it must match
// the name of the .desktop file (build/linux), which provides the icon.
const appID = "io.github.ndari.rho"

func main() {
	// Go source extensions run as "<rho binary> ext-host <path>".
	if len(os.Args) == 3 && os.Args[1] == "ext-host" {
		if err := yaegihost.Main(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, "rho-gui:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "rho-gui:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("rho-gui", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: rho-gui [flags] [directory]")
		fs.PrintDefaults()
	}
	var (
		cont     = fs.Bool("c", false, "continue the most recent session in the directory")
		model    = fs.String("m", "", "model id, or provider/name")
		sessionF = fs.String("session", "", "open a specific session file")
		noExt    = fs.Bool("no-extensions", false, "do not load extensions")
		showVer  = fs.Bool("version", false, "print the version and exit")
	)
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVer {
		fmt.Println("rho-gui", version)
		return nil
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("expected at most one directory, got %d arguments", fs.NArg())
	}
	cwd, err := workDir(fs.Arg(0))
	if err != nil {
		return err
	}

	opts := app.Options{
		Model:        *model,
		Version:      version,
		NoExtensions: *noExt,
	}
	if !*noExt {
		opts.ExtHost = extHost()
	}
	open := func(dir string, first bool) (*app.App, error) {
		o := opts
		o.Cwd = dir
		if first {
			o.Continue = *cont
			o.SessionFile = *sessionF
		}
		return app.New(o)
	}

	h := &host{}
	b := bridge.New(h, open, cwd)
	var restart *restartPlan
	if u, ok := updater(cwd); ok {
		u.Restart = func(res update.Result, args []string) {
			restart = &restartPlan{res: res, args: args}
			if a := h.app.Load(); a != nil {
				a.Quit()
			}
		}
		u.Args = func(dir, session string) []string {
			var args []string
			if session != "" {
				args = append(args, "-session", session)
			}
			if *noExt {
				args = append(args, "-no-extensions")
			}
			return append(args, dir)
		}
		bridge.EnableUpdates(b, u)
	}
	wapp := application.New(application.Options{
		Name:        "Rho",
		Description: "A minimal-core coding agent",
		Icon:        icon,
		Linux:       application.LinuxOptions{ApplicationID: appID},
		Services:    []application.Service{application.NewService(b)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	h.app.Store(wapp)
	h.win.Store(wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            bridge.WindowTitle(cwd),
		Width:            1100,
		Height:           780,
		MinWidth:         480,
		MinHeight:        360,
		BackgroundColour: application.NewRGB(29, 32, 33),
		URL:              "/",
	}))
	if err := wapp.Run(); err != nil {
		return err
	}
	if restart != nil {
		return restart.start()
	}
	return nil
}

// updater sets up update checks for release builds, unless they are
// turned off.
func updater(cwd string) (bridge.Updater, bool) {
	cur := update.Detect(version)
	// Only release builds update themselves; the GUI is not installed
	// with go install.
	if cur.Method != update.Archive {
		return bridge.Updater{}, false
	}
	paths, err := config.ResolvePaths(cwd)
	if err != nil {
		return bridge.Updater{}, false
	}
	// A broken config file still yields the settings read so far.
	cfg, _, _ := config.Load(paths)
	if !update.Enabled(cur, cfg) {
		return bridge.Updater{}, false
	}
	exe, err := os.Executable()
	if err != nil {
		return bridge.Updater{}, false
	}
	return bridge.Updater{Checker: &update.Checker{Current: cur, Home: paths.Home}, Exe: exe}, true
}

// restartPlan starts the updated GUI after this one has quit.
type restartPlan struct {
	res  update.Result
	args []string
}

// workDir picks the project directory: the argument, else the current
// directory, else (when started from a desktop launcher at /) the home
// directory.
func workDir(arg string) (string, error) {
	if arg != "" {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(abs)
		if err != nil {
			return "", err
		}
		if !info.IsDir() {
			return "", fmt.Errorf("%s is not a directory", arg)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil || cwd == "/" {
		if home, herr := os.UserHomeDir(); herr == nil {
			return home, nil
		}
	}
	return cwd, err
}

// extHost finds a rho binary to run Go source extensions: next to this
// executable, else on PATH when it has the same version. Starting rho-gui
// itself as the extension host works too, but loads the GUI libraries in
// every extension process. It returns "" (use this executable) when no
// matching rho is found.
func extHost() string {
	name := "rho"
	if runtime.GOOS == "windows" {
		name = "rho.exe"
	}
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			sibling := filepath.Join(filepath.Dir(exe), name)
			if info, err := os.Stat(sibling); err == nil && !info.IsDir() {
				return sibling
			}
		}
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	cmd := exec.Command(path, "--version")
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != "rho version "+version {
		return ""
	}
	return path
}

// host adapts the Wails application to bridge.Host.
type host struct {
	app atomic.Pointer[application.App]
	win atomic.Pointer[application.WebviewWindow]
}

func (h *host) Emit(name string, data any) {
	if a := h.app.Load(); a != nil {
		a.Event.Emit(name, data)
	}
}

func (h *host) SetClipboard(text string) bool {
	a := h.app.Load()
	return a != nil && a.Clipboard.SetText(text)
}

func (h *host) SetTitle(title string) {
	if w := h.win.Load(); w != nil {
		w.SetTitle(title)
	}
}

func (h *host) ChooseFolder(title, dir string) (string, error) {
	a := h.app.Load()
	if a == nil {
		return "", errors.New("the window is not ready")
	}
	d := a.Dialog.OpenFile().
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		SetTitle(title).
		SetDirectory(dir)
	if w := h.win.Load(); w != nil {
		d.AttachToWindow(w)
	}
	return d.PromptForSingleSelection()
}

func (h *host) OpenFile(path string) error {
	a := h.app.Load()
	if a == nil {
		return errors.New("the window is not ready")
	}
	return a.Browser.OpenFile(path)
}
