package bridge

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/NDari/rho/core/app"
	"github.com/NDari/rho/core/config"
	"github.com/NDari/rho/core/update"
)

func TestCreateConfig(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".rho")
	t.Setenv("RHO_HOME", home)
	open := func(dir string, first bool) (*app.App, error) {
		return app.New(app.Options{Cwd: dir, NoExtensions: true})
	}
	b := New(newFakeHost(), open, t.TempDir())
	t.Cleanup(b.close)

	info := b.Startup()
	if !strings.Contains(info.Error, "no model configured") || info.ConfigExists || info.Example != config.Example {
		t.Fatalf("startup = %+v", info)
	}
	if err := b.CreateConfig(); err != nil {
		t.Fatal(err)
	}
	if info := b.Startup(); info.Error != "" || !info.ConfigExists {
		t.Fatalf("startup after CreateConfig = %+v", info)
	}
	if got := b.State().Model; got != "local" {
		t.Errorf("model = %q", got)
	}
	if err := b.CreateConfig(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("second CreateConfig: %v", err)
	}
}

func TestUpdates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v9.9.9", "assets": []}`))
	}))
	t.Cleanup(srv.Close)
	b, h := newTestBridge(t)
	if b.UpdateAvailable() != "" {
		t.Fatal("an update is offered before checking")
	}
	var restarted []string
	EnableUpdates(b, Updater{
		Checker: &update.Checker{Current: update.Current{Version: "0.1.0", Method: update.Archive}, Home: t.TempDir(), API: srv.URL},
		Restart: func(_ update.Result, args []string) { restarted = args },
		Args:    func(dir, session string) []string { return []string{session, dir} },
	})
	deadline := time.Now().Add(5 * time.Second)
	for b.UpdateAvailable() == "" && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := b.UpdateAvailable(); got != "9.9.9" {
		t.Fatalf("UpdateAvailable = %q", got)
	}
	h.mu.Lock()
	sent := slices.Clone(h.updates)
	h.mu.Unlock()
	if !slices.Equal(sent, []string{"9.9.9"}) {
		t.Errorf("update events = %q", sent)
	}

	if err := b.Restart(); err == nil {
		t.Error("Restart worked before an update was installed")
	}
	// The release has no files for this platform.
	if _, err := b.InstallUpdate(); err == nil {
		t.Error("InstallUpdate worked without a download")
	}

	// A new session has no file yet, so only the folder is reopened.
	b.upd.installed = &update.Result{Version: "9.9.9"}
	if err := b.Restart(); err != nil {
		t.Fatal(err)
	}
	dir := b.State().Cwd
	if !slices.Equal(restarted, []string{"", dir}) {
		t.Errorf("restart args = %q", restarted)
	}
	if err := os.MkdirAll(filepath.Dir(b.State().SessionPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b.State().SessionPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.Restart(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(restarted, []string{b.State().SessionPath, dir}) {
		t.Errorf("restart args = %q", restarted)
	}
}
