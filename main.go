// Command rho-gui is Rho's desktop frontend.
package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/NDari/rho/core/app"
	"github.com/NDari/rho/core/ext/yaegihost"
	"github.com/NDari/rho/gui/bridge"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

//go:embed all:frontend/dist
var assets embed.FS

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

	a, err := app.New(app.Options{
		Cwd:          cwd,
		Model:        *model,
		Continue:     *cont,
		SessionFile:  *sessionF,
		Version:      version,
		NoExtensions: *noExt,
	})
	if err != nil {
		return err
	}

	h := &host{}
	b := bridge.New(a, h)
	wapp := application.New(application.Options{
		Name:        "Rho",
		Description: "A minimal-core coding agent",
		Services:    []application.Service{application.NewService(b)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
	})
	h.app.Store(wapp)
	wapp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Rho · " + filepath.Base(cwd),
		Width:            1100,
		Height:           780,
		MinWidth:         480,
		MinHeight:        360,
		BackgroundColour: application.NewRGB(29, 32, 33),
		URL:              "/",
	})
	return wapp.Run()
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

// host adapts the Wails application to bridge.Host.
type host struct {
	app atomic.Pointer[application.App]
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
