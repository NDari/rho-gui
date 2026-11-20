package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/NDari/rho/core/app"
	"github.com/NDari/rho/core/provider/fake"
)

// fakeHost records what the bridge sends to the frontend.
type fakeHost struct {
	mu        sync.Mutex
	events    []map[string]any
	uiReqs    chan UIRequest
	clipboard string
	title     string
	folder    string
	updates   []string
}

func newFakeHost() *fakeHost { return &fakeHost{uiReqs: make(chan UIRequest, 4)} }

func (h *fakeHost) Emit(name string, data any) {
	switch name {
	case EventBatch:
		h.mu.Lock()
		defer h.mu.Unlock()
		for _, raw := range data.([]json.RawMessage) {
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				panic(err)
			}
			h.events = append(h.events, m)
		}
	case EventUIRequest:
		h.uiReqs <- data.(UIRequest)
	case EventUpdate:
		h.mu.Lock()
		h.updates = append(h.updates, data.(string))
		h.mu.Unlock()
	}
}

func (h *fakeHost) SetClipboard(text string) bool {
	h.mu.Lock()
	h.clipboard = text
	h.mu.Unlock()
	return true
}

func (h *fakeHost) SetTitle(title string) {
	h.mu.Lock()
	h.title = title
	h.mu.Unlock()
}

func (h *fakeHost) ChooseFolder(title, dir string) (string, error) { return h.folder, nil }

func (h *fakeHost) OpenFile(path string) error { return nil }

// waitFor waits until an event of type typ arrived and returns all events.
func (h *fakeHost) waitFor(t *testing.T, typ string) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.Lock()
		for _, e := range h.events {
			if e["type"] == typ {
				out := append([]map[string]any(nil), h.events...)
				h.mu.Unlock()
				return out
			}
		}
		h.mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no %s event", typ)
	return nil
}

// testOpener opens projects with the fake provider answering steps.
func testOpener(steps ...fake.Step) Opener {
	return func(dir string, first bool) (*app.App, error) {
		a, err := app.New(app.Options{Cwd: dir, Model: "openai/test-model", NoExtensions: true})
		if err != nil {
			return nil, err
		}
		a.Registry.Set("openai", fake.New("openai", steps...))
		return a, nil
	}
}

func newTestBridge(t *testing.T, steps ...fake.Step) (*Bridge, *fakeHost) {
	t.Helper()
	t.Setenv("RHO_HOME", t.TempDir())
	h := newFakeHost()
	b := New(h, testOpener(steps...), t.TempDir())
	t.Cleanup(b.close)
	if info := b.Startup(); info.Error != "" {
		t.Fatal(info.Error)
	}
	return b, h
}

func TestSendStreamsEventsAndTranscript(t *testing.T) {
	b, h := newTestBridge(t, fake.Step{Thinking: "plan", Text: "hello there"})
	if err := b.Send("hi", false); err != nil {
		t.Fatal(err)
	}
	evs := h.waitFor(t, "turn_end")
	var types []string
	for _, e := range evs {
		types = append(types, e["type"].(string))
	}
	for _, want := range []string{"turn_start", "message_start", "text_delta", "message_end"} {
		found := false
		for _, typ := range types {
			found = found || typ == want
		}
		if !found {
			t.Errorf("missing %s event in %v", want, types)
		}
	}

	tr := b.Transcript()
	if len(tr.Items) != 2 || tr.Items[0].Text != "hi" || tr.Items[1].Text != "hello there" || tr.Items[1].Thinking != "plan" {
		t.Fatalf("transcript = %+v", tr.Items)
	}
	if s := b.State(); s.Busy || s.Model != "openai/test-model" {
		t.Errorf("state = %+v", s)
	}

	ok, err := b.Copy(false)
	if err != nil || !ok || h.clipboard != "hello there" {
		t.Errorf("Copy = %v, %v; clipboard %q", ok, err, h.clipboard)
	}
}

func TestSessionsAndBranching(t *testing.T) {
	b, h := newTestBridge(t, fake.Step{Text: "one"}, fake.Step{Text: "two"})
	for _, text := range []string{"first", "second"} {
		h.mu.Lock()
		h.events = nil
		h.mu.Unlock()
		if err := b.Send(text, false); err != nil {
			t.Fatal(err)
		}
		h.waitFor(t, "turn_end")
		b.core().Agent.Wait()
	}

	targets := b.RewindTargets()
	if len(targets) != 2 || targets[0].Text != "second" {
		t.Fatalf("rewind targets = %+v", targets)
	}
	text, err := b.Rewind(targets[0].ID)
	if err != nil || text != "second" {
		t.Fatalf("Rewind = %q, %v", text, err)
	}
	if n := len(b.Transcript().Items); n != 2 {
		t.Errorf("transcript after rewind has %d items, want 2", n)
	}

	path, err := b.Fork(b.ForkTargets()[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if b.State().SessionPath != path {
		t.Errorf("session path = %q, want the fork %q", b.State().SessionPath, path)
	}
	list, err := b.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("sessions = %+v", list)
	}
	if err := b.Label("", "here"); err != nil {
		t.Fatal(err)
	}
	if tree := b.Tree(); len(tree) == 0 || tree[len(tree)-1].Label != "here" {
		t.Errorf("tree = %+v", tree)
	}
}

func TestExtensionPrompts(t *testing.T) {
	b, h := newTestBridge(t)
	ui := extUI{b}

	tests := []struct {
		name    string
		ask     func() (string, error)
		value   string
		ok      bool
		want    string
		wantErr error
	}{
		{"confirm yes", func() (string, error) {
			ok, err := ui.Confirm(context.Background(), "Trust?", "a project extension")
			if ok {
				return "yes", err
			}
			return "no", err
		}, "", true, "yes", nil},
		{"select", func() (string, error) { return ui.Select(context.Background(), "Pick", []string{"a", "b"}) }, "b", true, "b", nil},
		{"input dismissed", func() (string, error) { return ui.Input(context.Background(), "Name", "type") }, "", false, "", ErrDismissed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			type result struct {
				v   string
				err error
			}
			done := make(chan result, 1)
			go func() {
				v, err := tc.ask()
				done <- result{v, err}
			}()
			req := <-h.uiReqs
			b.Respond(req.ID, tc.value, tc.ok)
			r := <-done
			if r.v != tc.want || !errors.Is(r.err, tc.wantErr) {
				t.Errorf("got %q, %v; want %q, %v", r.v, r.err, tc.want, tc.wantErr)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-h.uiReqs
		cancel()
	}()
	if _, err := ui.Confirm(ctx, "t", "m"); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled prompt error = %v", err)
	}
}

func TestSettingsLeaveOutTUIOnlyKeys(t *testing.T) {
	b, _ := newTestBridge(t)
	keys := map[string]bool{}
	for _, s := range b.Settings() {
		keys[s.Key] = true
	}
	if keys["theme"] || keys["tui.mouse_select"] || !keys["tui.show_thinking"] || !keys["max_retries"] {
		t.Errorf("settings = %v", keys)
	}
}

func TestStartupErrorAndRetry(t *testing.T) {
	home := t.TempDir()
	t.Setenv("RHO_HOME", home)
	cfg := filepath.Join(home, "config.toml")
	if err := os.WriteFile(cfg, []byte("not [valid toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := newFakeHost()
	dir := t.TempDir()
	b := New(h, testOpener(), dir)
	t.Cleanup(b.close)

	info := b.Startup()
	if info.Error == "" || info.Dir != dir || info.ConfigPath != cfg {
		t.Fatalf("startup = %+v", info)
	}
	if err := b.Retry(); err == nil {
		t.Fatal("Retry succeeded with a broken config")
	}
	if err := os.Remove(cfg); err != nil {
		t.Fatal(err)
	}
	if err := b.Retry(); err != nil {
		t.Fatal(err)
	}
	if info := b.Startup(); info.Error != "" {
		t.Errorf("startup after retry = %+v", info)
	}
	if b.State().Cwd != dir || h.title != WindowTitle(dir) {
		t.Errorf("cwd %q, title %q", b.State().Cwd, h.title)
	}
}

func TestOpenAndFiles(t *testing.T) {
	b, h := newTestBridge(t)
	first := b.State().SessionPath

	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "README.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.folder = other
	dir, err := b.ChooseFolder()
	if err != nil || dir != other {
		t.Fatalf("ChooseFolder = %q, %v", dir, err)
	}
	if err := b.Open(filepath.Join(other, "README.md")); err == nil {
		t.Error("Open accepted a file")
	}
	if err := b.Open(dir); err != nil {
		t.Fatal(err)
	}
	s := b.State()
	if s.Cwd != other || s.SessionPath == first || h.title != WindowTitle(other) {
		t.Errorf("after Open: state %+v, title %q", s, h.title)
	}
	if got := b.Files("RE"); len(got) != 1 || got[0] != "README.md" {
		t.Errorf("Files = %q", got)
	}
}
