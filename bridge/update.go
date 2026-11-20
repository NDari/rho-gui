package bridge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/NDari/rho/core/config"
	"github.com/NDari/rho/core/update"
)

// EventUpdate tells the frontend that a newer version is available. Its
// data is the version.
const EventUpdate = "rho:update"

// Updater lets the bridge install updates. The GUI's main package
// provides it when update checks are on.
type Updater struct {
	// Checker looks for and installs releases.
	Checker *update.Checker
	// Exe is the running program.
	Exe string
	// Restart quits the GUI and starts the installed update with args.
	Restart func(res update.Result, args []string)
	// Args are the command-line arguments that reopen dir, and session
	// when it is not empty.
	Args func(dir, session string) []string
}

type updates struct {
	mu        sync.Mutex
	u         *Updater
	latest    string
	installed *update.Result
}

// EnableUpdates turns on update checks for b: a version found by an
// earlier check is offered at once, and a fresh check (at most once a day)
// runs in the background. It is a function, not a method, so that it is
// not bound to the frontend.
func EnableUpdates(b *Bridge, u Updater) {
	b.upd.mu.Lock()
	b.upd.u = &u
	b.upd.latest = u.Checker.Cached()
	b.upd.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		latest, err := u.Checker.Check(ctx)
		if err != nil || latest == "" {
			return
		}
		b.upd.mu.Lock()
		changed := latest != b.upd.latest
		b.upd.latest = latest
		b.upd.mu.Unlock()
		if changed {
			b.host.Emit(EventUpdate, latest)
		}
	}()
}

// UpdateAvailable returns the newer version that can be installed, or "".
func (b *Bridge) UpdateAvailable() string {
	b.upd.mu.Lock()
	defer b.upd.mu.Unlock()
	return b.upd.latest
}

// UpdateResult describes an installed update.
type UpdateResult struct {
	Version string `json:"version"`
	// Notes tell the user about anything left to do, such as adding a
	// folder to PATH.
	Notes []string `json:"notes"`
}

// InstallUpdate downloads and installs the latest release. Restart starts
// it afterwards.
func (b *Bridge) InstallUpdate() (UpdateResult, error) {
	b.upd.mu.Lock()
	u := b.upd.u
	b.upd.mu.Unlock()
	if u == nil {
		return UpdateResult{}, errors.New("updates are turned off")
	}
	if a := b.core(); a != nil && a.Agent.Busy() {
		return UpdateResult{}, errors.New("the agent is working; wait for it to finish or cancel it first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	rel, err := u.Checker.Latest(ctx)
	if err != nil {
		return UpdateResult{}, err
	}
	if !update.Newer(rel.Version, u.Checker.Current.Version) {
		return UpdateResult{}, fmt.Errorf("version %s is already the latest", u.Checker.Current.Version)
	}
	res, err := u.Checker.Install(ctx, rel, u.Exe)
	if err != nil {
		return UpdateResult{}, err
	}
	b.upd.mu.Lock()
	b.upd.installed = &res
	b.upd.mu.Unlock()
	return UpdateResult{Version: res.Version, Notes: res.Notes}, nil
}

// Restart quits and starts the installed update in the same folder, with
// the open session.
func (b *Bridge) Restart() error {
	b.upd.mu.Lock()
	u, res := b.upd.u, b.upd.installed
	b.upd.mu.Unlock()
	if u == nil || res == nil {
		return errors.New("no update is installed")
	}
	b.switching.Lock()
	dir, session := b.dir, ""
	if a := b.core(); a != nil {
		if a.Agent.Busy() {
			b.switching.Unlock()
			return errors.New("the agent is working; wait for it to finish or cancel it first")
		}
		// A new session has no file until its first message.
		if p := a.Agent.Session().Path(); fileExists(p) {
			session = p
		}
	}
	b.switching.Unlock()
	u.Restart(*res, u.Args(dir, session))
	return nil
}

// CreateConfig writes the example configuration to the global config
// file, which must not exist yet, and opens the project again.
func (b *Bridge) CreateConfig() error {
	home, err := config.HomeDir()
	if err != nil {
		return err
	}
	if err := config.CreateExample(filepath.Join(home, "config.toml")); err != nil {
		return err
	}
	return b.Retry()
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
