// Package bridge connects the Rho core to the GUI frontend. Its exported
// methods are bound to the frontend by Wails; core events are forwarded in
// batches through a Host. The package does not import Wails, so it can be
// tested without a webview.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/NDari/rho/core/app"
	"github.com/NDari/rho/core/config"
	"github.com/NDari/rho/core/events"
	"github.com/NDari/rho/core/ext"
	"github.com/NDari/rho/core/prompt"
	"github.com/NDari/rho/core/provider"
	"github.com/NDari/rho/core/session"
)

// Event names sent to the frontend.
const (
	// EventBatch carries a JSON array of core events (see events.Marshal).
	EventBatch = "rho:events"
	// EventUIRequest asks the frontend to show an extension prompt.
	EventUIRequest = "rho:ui"
)

// flushInterval batches streamed events to about the display frame rate.
const flushInterval = 16 * time.Millisecond

// Host is the part of the GUI runtime the bridge needs.
type Host interface {
	// Emit sends an event to the frontend.
	Emit(name string, data any)
	// SetClipboard copies text to the system clipboard.
	SetClipboard(text string) bool
	// SetTitle sets the window title.
	SetTitle(title string)
	// ChooseFolder asks the user for a directory, starting in dir. It
	// returns "" when the user cancels.
	ChooseFolder(title, dir string) (string, error)
	// OpenFile opens a file with the system's default application.
	OpenFile(path string) error
}

// Opener creates the core for a project directory. first is true when
// opening the directory the GUI was started with, so that start-up flags
// such as -c apply only to it.
type Opener func(dir string, first bool) (*app.App, error)

// ErrDismissed is returned when the user dismisses an extension prompt.
var ErrDismissed = errors.New("dismissed by the user")

// Bridge is the service bound to the frontend.
type Bridge struct {
	host Host
	open Opener

	// cur is the core for the open project. It is nil when opening the
	// first directory failed.
	cur atomic.Pointer[app.App]
	// started is the core whose extensions were started.
	started atomic.Pointer[app.App]

	// switching serializes Open, Retry and close.
	switching sync.Mutex
	dir       string // the directory last opened (or tried)
	startErr  string // why opening the first directory failed

	mu      sync.Mutex
	pending []json.RawMessage
	asks    map[int64]chan reply
	nextAsk int64

	upd updates

	stop chan struct{}
	done chan struct{}
}

type reply struct {
	value string
	ok    bool
}

// New creates a bridge that opens dir with open and sends events to host.
// When opening fails the bridge still works: Startup reports the error and
// Retry or Open try again. The GUI runtime calls ServiceShutdown when it
// exits.
func New(host Host, open Opener, dir string) *Bridge {
	b := &Bridge{
		host: host,
		open: open,
		dir:  dir,
		asks: map[int64]chan reply{},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	if a, err := open(dir, true); err != nil {
		b.startErr = err.Error()
	} else {
		b.use(a)
	}
	go b.flushLoop()
	return b
}

// core returns the core for the open project.
func (b *Bridge) core() *app.App { return b.cur.Load() }

// use makes a the open project's core.
func (b *Bridge) use(a *app.App) {
	a.Agent.SetSink(b.push)
	b.cur.Store(a)
	b.dir = a.Paths.Cwd
	b.startErr = ""
}

func (b *Bridge) push(e events.Event) {
	data, err := events.Marshal(e)
	if err != nil {
		return
	}
	b.mu.Lock()
	b.pending = append(b.pending, data)
	b.mu.Unlock()
}

func (b *Bridge) flushLoop() {
	defer close(b.done)
	t := time.NewTicker(flushInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			b.flush()
		case <-b.stop:
			b.flush()
			return
		}
	}
}

func (b *Bridge) flush() {
	b.mu.Lock()
	batch := b.pending
	b.pending = nil
	b.mu.Unlock()
	if len(batch) > 0 {
		b.host.Emit(EventBatch, batch)
	}
}

// ServiceShutdown is called by Wails when the GUI exits. It is not bound
// to the frontend.
func (b *Bridge) ServiceShutdown() error {
	b.close()
	return nil
}

// close shuts down the open project and stops forwarding events.
func (b *Bridge) close() {
	b.switching.Lock()
	defer b.switching.Unlock()
	if a := b.cur.Swap(nil); a != nil {
		shutdown(a)
	}
	select {
	case <-b.stop:
	default:
		close(b.stop)
	}
	<-b.done
}

// shutdown cancels any running turn, stops extensions and closes the
// session.
func shutdown(a *app.App) {
	a.Agent.Cancel()
	a.Agent.Wait()
	a.StopExtensions()
	_ = a.Agent.Session().Close()
}

// StartupInfo tells the frontend whether a project is open.
type StartupInfo struct {
	// Error is why the project could not be opened; empty when it is open.
	Error string `json:"error,omitempty"`
	// Dir is the project directory (or the one that failed to open).
	Dir string `json:"dir"`
	// ConfigPath is the global configuration file.
	ConfigPath string `json:"config_path"`
	// ConfigExists reports whether ConfigPath exists.
	ConfigExists bool `json:"config_exists"`
	// Example is a starting configuration for a local server.
	Example string `json:"example"`
}

// Startup reports whether a project is open. The frontend calls it before
// anything else; when Error is set, only Startup, Retry, Open,
// ChooseFolder, OpenConfig, CreateConfig, HomeDir and the update methods
// may be used.
func (b *Bridge) Startup() StartupInfo {
	b.switching.Lock()
	defer b.switching.Unlock()
	info := StartupInfo{Error: b.startErr, Dir: b.dir, Example: config.Example}
	if home, err := config.HomeDir(); err == nil {
		info.ConfigPath = filepath.Join(home, "config.toml")
		_, err := os.Stat(info.ConfigPath)
		info.ConfigExists = err == nil
	}
	return info
}

// Retry opens the start-up directory again with the start-up flags, after
// the user fixed what made it fail.
func (b *Bridge) Retry() error {
	b.switching.Lock()
	defer b.switching.Unlock()
	if b.cur.Load() != nil {
		return nil
	}
	a, err := b.open(b.dir, true)
	if err != nil {
		b.startErr = err.Error()
		return err
	}
	b.use(a)
	b.setTitle()
	return nil
}

// Open switches to another project directory with a new session. The
// open project is left as it is when the new one fails to open. The
// frontend reloads afterwards.
func (b *Bridge) Open(dir string) error {
	b.switching.Lock()
	defer b.switching.Unlock()
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	old := b.cur.Load()
	if old != nil && old.Agent.Busy() {
		return errors.New("the agent is working; cancel it first")
	}
	a, err := b.open(dir, false)
	if err != nil {
		return err
	}
	if old != nil {
		b.dismissAsks()
		shutdown(old)
	}
	b.use(a)
	b.setTitle()
	return nil
}

func (b *Bridge) setTitle() { b.host.SetTitle(WindowTitle(b.dir)) }

// WindowTitle is the window title for a project directory.
func WindowTitle(dir string) string { return "Rho · " + filepath.Base(dir) }

// ChooseFolder asks the user for a project directory. It returns "" when
// the user cancels.
func (b *Bridge) ChooseFolder() (string, error) {
	b.switching.Lock()
	dir := b.dir
	b.switching.Unlock()
	return b.host.ChooseFolder("Open a project folder", dir)
}

// OpenConfig opens the global configuration file in the default editor.
func (b *Bridge) OpenConfig() error {
	info := b.Startup()
	if !info.ConfigExists {
		return fmt.Errorf("%s does not exist", info.ConfigPath)
	}
	return b.host.OpenFile(info.ConfigPath)
}

// Files lists completions for an @file reference being typed (see
// app.FileCompletions).
func (b *Bridge) Files(partial string) []string {
	return app.FileCompletions(b.core().Paths.Cwd, partial)
}

// State is a snapshot of what the frontend shows around the transcript.
type State struct {
	Version       string       `json:"version"`
	Cwd           string       `json:"cwd"`
	Branch        string       `json:"branch,omitempty"`
	Model         string       `json:"model"`
	ContextWindow int          `json:"context_window"`
	ContextTokens int          `json:"context_tokens"`
	SessionCost   float64      `json:"session_cost"`
	SessionPath   string       `json:"session_path"`
	Busy          bool         `json:"busy"`
	Aside         bool         `json:"aside"`
	ShowThinking  bool         `json:"show_thinking"`
	Extensions    bool         `json:"extensions"`
	Warnings      []string     `json:"warnings,omitempty"`
	Queue         events.Queue `json:"queue"`
}

// State returns the current state.
func (b *Bridge) State() State {
	a := b.core()
	ag := a.Agent
	cv := ag.Session().BuildContext()
	return State{
		Version:       a.Version,
		Cwd:           a.Paths.Cwd,
		Branch:        app.GitBranch(a.Paths.ProjectRoot),
		Model:         ag.Model().ID,
		ContextWindow: ag.Model().ContextWindow,
		ContextTokens: ag.EstimateContext(cv),
		SessionCost:   ag.Cost(),
		SessionPath:   ag.Session().Path(),
		Busy:          ag.Busy(),
		Aside:         cv.Aside,
		ShowThinking:  ag.Config().TUI.ShowThinking,
		Extensions:    a.Ext != nil,
		Warnings:      a.Warnings,
	}
}

// Ready is called once the frontend can show prompts. It starts the open
// project's extensions (once), which may ask the user to approve project
// extensions.
func (b *Bridge) Ready() {
	a := b.core()
	if a == nil || b.started.Swap(a) == a {
		return
	}
	if a.Ext != nil {
		go a.StartExtensions(context.Background(), extUI{b})
	}
}

// Transcript returns the visible conversation on the current path.
func (b *Bridge) Transcript() app.Transcript { return b.core().Transcript() }

// Send sends a message. While the agent works, the message steers it (or
// is queued as a follow-up). @file references are attached.
func (b *Bridge) Send(text string, followUp bool) error {
	ag := b.core().Agent
	if ag.Busy() {
		if followUp {
			return ag.FollowUp(text)
		}
		return ag.Steer(text)
	}
	return ag.Prompt(b.core().BuildUserMessage(text))
}

// TakeQueue returns and clears messages still queued when a turn ended.
func (b *Bridge) TakeQueue() []string { return b.core().Agent.TakeQueue() }

// Cancel stops the running turn.
func (b *Bridge) Cancel() { b.core().Agent.Cancel() }

// Command is a slash command from a prompt template or an extension.
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Source is "template" or the extension's name.
	Source string `json:"source"`
}

// Commands lists the prompt template and extension commands.
func (b *Bridge) Commands() []Command {
	var out []Command
	for _, t := range b.core().Templates {
		out = append(out, Command{Name: t.Name, Description: t.Description, Source: "template"})
	}
	if b.core().Ext != nil {
		for _, c := range b.core().Ext.Commands() {
			out = append(out, Command{Name: c.Name, Description: c.Description, Source: c.Extension})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// RunCommand runs a prompt template or extension command. It reports
// false when no such command exists.
func (b *Bridge) RunCommand(ctx context.Context, name, args string, followUp bool) (bool, error) {
	for _, t := range b.core().Templates {
		if t.Name == name {
			return true, b.Send(t.Expand(args), followUp)
		}
	}
	if b.core().Ext != nil {
		for _, c := range b.core().Ext.Commands() {
			if c.Name == name {
				return true, b.core().Ext.RunCommand(ctx, name, args)
			}
		}
	}
	return false, nil
}

// ModelInfo describes a configured model.
type ModelInfo struct {
	ID            string `json:"id"`
	Provider      string `json:"provider"`
	ContextWindow int    `json:"context_window"`
}

// Models lists the configured models.
func (b *Bridge) Models() []ModelInfo {
	var out []ModelInfo
	for _, m := range b.core().Agent.Config().Models {
		out = append(out, ModelInfo{ID: m.ID, Provider: m.Provider, ContextWindow: m.ContextWindow})
	}
	return out
}

// SetModel switches the model ("provider/name" or a configured ID).
func (b *Bridge) SetModel(id string) error { return b.core().Agent.SetModel(id) }

// NewSession starts a new session.
func (b *Bridge) NewSession() error { return b.core().Agent.SetSession(b.core().NewSession()) }

// SessionInfo describes a saved session.
type SessionInfo struct {
	Path     string    `json:"path"`
	Title    string    `json:"title"`
	Model    string    `json:"model,omitempty"`
	Messages int       `json:"messages"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
	Current  bool      `json:"current,omitempty"`
}

// Sessions lists the sessions in the working directory, newest first.
func (b *Bridge) Sessions() ([]SessionInfo, error) {
	list, err := session.List(b.core().SessionDir())
	if err != nil {
		return nil, err
	}
	cur := b.core().Agent.Session().Path()
	out := make([]SessionInfo, 0, len(list))
	for _, s := range list {
		out = append(out, SessionInfo{
			Path: s.Path, Title: s.Title, Model: s.Model, Messages: s.Messages,
			Size: s.Size, Modified: s.Modified, Current: s.Path == cur,
		})
	}
	return out, nil
}

// Resume opens a saved session.
func (b *Bridge) Resume(path string) error {
	s, err := session.Open(path)
	if err != nil {
		return err
	}
	return b.core().Agent.SetSession(s)
}

// RewindTargets lists the user messages that /rewind can go back to.
func (b *Bridge) RewindTargets() []app.MessageRef { return b.core().PathMessages(provider.RoleUser) }

// Rewind moves the conversation to before entry id and returns that
// message's text for editing. Files on disk are not changed.
func (b *Bridge) Rewind(id string) (string, error) { return b.core().Agent.RewindTo(id) }

// ForkTargets lists the positions /fork can copy the conversation up to.
// The first entry is the current position when that is not a message.
func (b *Bridge) ForkTargets() []app.MessageRef {
	refs := b.core().PathMessages(provider.RoleUser, provider.RoleAssistant)
	if leaf := b.core().Agent.Session().Leaf(); leaf != "" && (len(refs) == 0 || refs[0].ID != leaf) {
		refs = append([]app.MessageRef{{ID: leaf, Text: "Current position (everything so far)"}}, refs...)
	}
	return refs
}

// Fork copies the conversation up to entry id into a new session and
// switches to it. It returns the new session's path.
func (b *Bridge) Fork(id string) (string, error) {
	s, err := b.core().Fork(id)
	if err != nil {
		return "", err
	}
	if err := b.core().Agent.SetSession(s); err != nil {
		return "", err
	}
	return s.Path(), nil
}

// Tree lists the session tree.
func (b *Bridge) Tree() []app.TreeNode { return b.core().Tree() }

// Checkout moves to another entry of the session tree.
func (b *Bridge) Checkout(id string) error { return b.core().Agent.Checkout(id) }

// Label sets (or with an empty label removes) the label of entry id; an
// empty id labels the current position.
func (b *Bridge) Label(id, label string) error {
	if id == "" {
		id = b.core().Agent.Session().Leaf()
		if id == "" {
			return errors.New("nothing to label yet")
		}
	}
	return b.core().Agent.Label(id, label)
}

// StartAside starts a read-only side conversation.
func (b *Bridge) StartAside() error { return b.core().Agent.StartAside() }

// EndAside returns from a side conversation, carrying an optional note.
func (b *Bridge) EndAside(note string) error { return b.core().Agent.EndAside(note) }

// SummarizeAside drafts a note summarizing the side conversation.
func (b *Bridge) SummarizeAside(ctx context.Context) (string, error) {
	return b.core().Agent.SummarizeAside(ctx)
}

// Compact summarizes older messages to free context.
func (b *Bridge) Compact(ctx context.Context) error { return b.core().Agent.Compact(ctx) }

// CompactionSummary returns the summary stored in a compaction entry.
func (b *Bridge) CompactionSummary(entryID string) string {
	e, _ := b.core().Agent.Session().Get(entryID)
	return e.Summary
}

// ContextPart is one part of the system prompt.
type ContextPart struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Tokens int    `json:"tokens"`
}

// ContextInfo reports what is sent to the model (token estimates).
type ContextInfo struct {
	Parts         []ContextPart `json:"parts"`
	Tools         int           `json:"tools"`
	ToolTokens    int           `json:"tool_tokens"`
	Messages      int           `json:"messages"`
	MessageTokens int           `json:"message_tokens"`
	Total         int           `json:"total"`
	Window        int           `json:"window"`
	Compacted     bool          `json:"compacted,omitempty"`
	Skills        []string      `json:"skills,omitempty"`
}

// Context reports what is sent to the model.
func (b *Bridge) Context() (ContextInfo, error) {
	r, err := b.core().Agent.ContextReport()
	if err != nil {
		return ContextInfo{}, err
	}
	info := ContextInfo{
		Tools: len(b.core().Agent.Tools()), ToolTokens: r.ToolTokens,
		Messages: r.Messages, MessageTokens: r.MessageTokens,
		Total: r.Total, Window: r.Window, Compacted: r.Compacted,
	}
	for _, p := range r.Parts {
		info.Parts = append(info.Parts, ContextPart{Name: p.Name, Path: p.Path, Tokens: p.Tokens})
	}
	for _, s := range b.core().Skills {
		info.Skills = append(info.Skills, s.Name)
	}
	return info, nil
}

// Copy copies the last answer (or with all the whole conversation) to the
// clipboard. It reports false when there is nothing to copy.
func (b *Bridge) Copy(all bool) (bool, error) {
	text := b.core().CopyText(all)
	if text == "" {
		return false, nil
	}
	if !b.host.SetClipboard(text) {
		return false, errors.New("could not use the clipboard")
	}
	return true, nil
}

// SettingInfo describes a setting that can change while Rho runs.
type SettingInfo struct {
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Value       string   `json:"value"`
	Choices     []string `json:"choices,omitempty"`
	// Kind is "string", "int" or "bool".
	Kind     string `json:"kind"`
	PerModel bool   `json:"per_model,omitempty"`
	Optional bool   `json:"optional,omitempty"`
}

// guiSettings leaves out settings that only affect the TUI.
func guiSetting(key string) bool {
	return key != "theme" && (!strings.HasPrefix(key, "tui.") || key == "tui.show_thinking")
}

// Settings lists the settings the GUI can change.
func (b *Bridge) Settings() []SettingInfo {
	cfg, model := b.core().Agent.Config(), b.core().Agent.Model().ID
	var out []SettingInfo
	for _, s := range config.Settings {
		if !guiSetting(s.Key) {
			continue
		}
		choices := s.Choices
		if s.Key == "default_model" {
			choices = nil
			for _, m := range cfg.Models {
				choices = append(choices, m.ID)
			}
		}
		kind := "string"
		switch s.Kind {
		case config.KindInt:
			kind = "int"
		case config.KindBool:
			kind = "bool"
		}
		out = append(out, SettingInfo{
			Key: s.Key, Description: s.Description, Value: s.Value(cfg, model),
			Choices: choices, Kind: kind, PerModel: s.PerModel, Optional: s.Optional,
		})
	}
	return out
}

// SettingResult reports how a setting change was kept.
type SettingResult struct {
	// Saved is false when the value applies to this session only.
	Saved bool   `json:"saved"`
	Note  string `json:"note,omitempty"`
	Value string `json:"value"`
}

// SetSetting changes a setting and saves it to the global config.
func (b *Bridge) SetSetting(key, value string) (SettingResult, error) {
	saved, note, err := b.core().SetSetting(key, value)
	if err != nil {
		return SettingResult{}, err
	}
	s, _ := config.LookupSetting(key)
	return SettingResult{Saved: saved, Note: note, Value: s.Value(b.core().Agent.Config(), b.core().Agent.Model().ID)}, nil
}

// ExtensionInfo describes a discovered extension.
type ExtensionInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Project  bool   `json:"project,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	Tools    int    `json:"tools"`
	Commands int    `json:"commands"`
	Hooks    int    `json:"hooks"`
	Disabled bool   `json:"disabled,omitempty"`
}

// Extensions lists the discovered extensions.
func (b *Bridge) Extensions() []ExtensionInfo {
	if b.core().Ext == nil {
		return nil
	}
	var out []ExtensionInfo
	for _, x := range b.core().Ext.Extensions() {
		x.Lock()
		out = append(out, ExtensionInfo{
			ID: x.ID, Name: x.Name, Kind: x.Kind, Project: x.Project,
			Status: x.Status, Error: x.Err,
			Tools: len(x.Info.Tools), Commands: len(x.Info.Commands), Hooks: len(x.Info.Hooks),
			Disabled: x.Status == ext.StatusDisabled,
		})
		x.Unlock()
	}
	return out
}

// SetExtensionEnabled starts or stops an extension and saves the choice.
// The note, if any, warns that the choice won't hold on the next start.
func (b *Bridge) SetExtensionEnabled(ctx context.Context, id string, on bool) (string, error) {
	return b.core().SetExtensionEnabled(ctx, id, on)
}

// Reload reloads skills, prompt templates and context files and restarts
// the extensions.
func (b *Bridge) Reload(ctx context.Context) error {
	skills, w1 := prompt.LoadSkills(b.core().Paths)
	templates, w2 := prompt.LoadTemplates(b.core().Paths)
	b.core().Skills = skills
	b.core().Templates = templates
	b.core().Agent.SetSkills(skills)
	for _, w := range slices.Concat(w1, w2) {
		b.push(events.Notice{Level: "warn", Text: w})
	}
	if err := b.core().ReloadExtensions(ctx); err != nil {
		return err
	}
	what := "skills, prompt templates and context files"
	if b.core().Ext != nil {
		what = "extensions, " + what
	}
	b.push(events.Notice{Level: "info", Text: "reloaded " + what})
	return nil
}

// HomeDir is the user's home directory, for shortening paths.
func (b *Bridge) HomeDir() string {
	h, _ := os.UserHomeDir()
	return h
}

// UIRequest asks the frontend to show an extension prompt. The answer is
// sent back with Respond.
type UIRequest struct {
	ID int64 `json:"id"`
	// Kind is "confirm", "select" or "input".
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	Message     string   `json:"message,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Options     []string `json:"options,omitempty"`
}

// Respond answers the extension prompt with the given ID. ok is false when
// the user dismissed it.
func (b *Bridge) Respond(id int64, value string, ok bool) {
	b.mu.Lock()
	ch := b.asks[id]
	delete(b.asks, id)
	b.mu.Unlock()
	if ch != nil {
		ch <- reply{value: value, ok: ok}
	}
}

// dismissAsks answers all open extension prompts as dismissed.
func (b *Bridge) dismissAsks() {
	b.mu.Lock()
	asks := b.asks
	b.asks = map[int64]chan reply{}
	b.mu.Unlock()
	for _, ch := range asks {
		ch <- reply{}
	}
}

func (b *Bridge) ask(ctx context.Context, r UIRequest) (reply, error) {
	ch := make(chan reply, 1)
	b.mu.Lock()
	b.nextAsk++
	r.ID = b.nextAsk
	b.asks[r.ID] = ch
	b.mu.Unlock()
	// Deliver queued events first so the prompt shows after them.
	b.flush()
	b.host.Emit(EventUIRequest, r)
	select {
	case rep := <-ch:
		return rep, nil
	case <-ctx.Done():
		b.mu.Lock()
		delete(b.asks, r.ID)
		b.mu.Unlock()
		return reply{}, ctx.Err()
	}
}

// extUI implements ext.UI. It is a separate type so its methods are not
// bound to the frontend.
type extUI struct{ b *Bridge }

var _ ext.UI = extUI{}

func (u extUI) Confirm(ctx context.Context, title, message string) (bool, error) {
	rep, err := u.b.ask(ctx, UIRequest{Kind: "confirm", Title: title, Message: message})
	return rep.ok, err
}

func (u extUI) Select(ctx context.Context, title string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("select %q: no options", title)
	}
	rep, err := u.b.ask(ctx, UIRequest{Kind: "select", Title: title, Options: options})
	if err == nil && !rep.ok {
		err = ErrDismissed
	}
	return rep.value, err
}

func (u extUI) Input(ctx context.Context, title, placeholder string) (string, error) {
	rep, err := u.b.ask(ctx, UIRequest{Kind: "input", Title: title, Placeholder: placeholder})
	if err == nil && !rep.ok {
		err = ErrDismissed
	}
	return rep.value, err
}

func (u extUI) Notify(level, text string) { u.b.push(events.Notice{Level: level, Text: text}) }

func (u extUI) SetStatus(key, text string) { u.b.push(events.Status{Key: key, Text: text}) }
