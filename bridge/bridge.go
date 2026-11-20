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
	"slices"
	"sort"
	"strings"
	"sync"
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
}

// ErrDismissed is returned when the user dismisses an extension prompt.
var ErrDismissed = errors.New("dismissed by the user")

// Bridge is the service bound to the frontend.
type Bridge struct {
	app  *app.App
	host Host

	mu      sync.Mutex
	pending []json.RawMessage
	asks    map[int64]chan reply
	nextAsk int64

	ready sync.Once
	stop  chan struct{}
	done  chan struct{}
}

type reply struct {
	value string
	ok    bool
}

// New creates a bridge for a, sending events to host. The GUI runtime
// calls ServiceShutdown when it exits.
func New(a *app.App, host Host) *Bridge {
	b := &Bridge{
		app:  a,
		host: host,
		asks: map[int64]chan reply{},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	a.Agent.SetSink(b.push)
	go b.flushLoop()
	return b
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

// close cancels any running turn, stops extensions and closes the session.
func (b *Bridge) close() {
	b.app.Agent.Cancel()
	b.app.Agent.Wait()
	b.app.StopExtensions()
	_ = b.app.Agent.Session().Close()
	select {
	case <-b.stop:
	default:
		close(b.stop)
	}
	<-b.done
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
	ag := b.app.Agent
	cv := ag.Session().BuildContext()
	return State{
		Version:       b.app.Version,
		Cwd:           b.app.Paths.Cwd,
		Branch:        app.GitBranch(b.app.Paths.ProjectRoot),
		Model:         ag.Model().ID,
		ContextWindow: ag.Model().ContextWindow,
		ContextTokens: ag.EstimateContext(cv),
		SessionCost:   ag.Cost(),
		SessionPath:   ag.Session().Path(),
		Busy:          ag.Busy(),
		Aside:         cv.Aside,
		ShowThinking:  ag.Config().TUI.ShowThinking,
		Extensions:    b.app.Ext != nil,
		Warnings:      b.app.Warnings,
	}
}

// Ready is called once the frontend can show prompts. It starts the
// extensions, which may ask the user to approve project extensions.
func (b *Bridge) Ready() {
	b.ready.Do(func() {
		if b.app.Ext != nil {
			go b.app.StartExtensions(context.Background(), extUI{b})
		}
	})
}

// Transcript returns the visible conversation on the current path.
func (b *Bridge) Transcript() app.Transcript { return b.app.Transcript() }

// Send sends a message. While the agent works, the message steers it (or
// is queued as a follow-up). @file references are attached.
func (b *Bridge) Send(text string, followUp bool) error {
	ag := b.app.Agent
	if ag.Busy() {
		if followUp {
			return ag.FollowUp(text)
		}
		return ag.Steer(text)
	}
	return ag.Prompt(b.app.BuildUserMessage(text))
}

// TakeQueue returns and clears messages still queued when a turn ended.
func (b *Bridge) TakeQueue() []string { return b.app.Agent.TakeQueue() }

// Cancel stops the running turn.
func (b *Bridge) Cancel() { b.app.Agent.Cancel() }

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
	for _, t := range b.app.Templates {
		out = append(out, Command{Name: t.Name, Description: t.Description, Source: "template"})
	}
	if b.app.Ext != nil {
		for _, c := range b.app.Ext.Commands() {
			out = append(out, Command{Name: c.Name, Description: c.Description, Source: c.Extension})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// RunCommand runs a prompt template or extension command. It reports
// false when no such command exists.
func (b *Bridge) RunCommand(ctx context.Context, name, args string, followUp bool) (bool, error) {
	for _, t := range b.app.Templates {
		if t.Name == name {
			return true, b.Send(t.Expand(args), followUp)
		}
	}
	if b.app.Ext != nil {
		for _, c := range b.app.Ext.Commands() {
			if c.Name == name {
				return true, b.app.Ext.RunCommand(ctx, name, args)
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
	for _, m := range b.app.Agent.Config().Models {
		out = append(out, ModelInfo{ID: m.ID, Provider: m.Provider, ContextWindow: m.ContextWindow})
	}
	return out
}

// SetModel switches the model ("provider/name" or a configured ID).
func (b *Bridge) SetModel(id string) error { return b.app.Agent.SetModel(id) }

// NewSession starts a new session.
func (b *Bridge) NewSession() error { return b.app.Agent.SetSession(b.app.NewSession()) }

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
	list, err := session.List(b.app.SessionDir())
	if err != nil {
		return nil, err
	}
	cur := b.app.Agent.Session().Path()
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
	return b.app.Agent.SetSession(s)
}

// RewindTargets lists the user messages that /rewind can go back to.
func (b *Bridge) RewindTargets() []app.MessageRef { return b.app.PathMessages(provider.RoleUser) }

// Rewind moves the conversation to before entry id and returns that
// message's text for editing. Files on disk are not changed.
func (b *Bridge) Rewind(id string) (string, error) { return b.app.Agent.RewindTo(id) }

// ForkTargets lists the positions /fork can copy the conversation up to.
// The first entry is the current position when that is not a message.
func (b *Bridge) ForkTargets() []app.MessageRef {
	refs := b.app.PathMessages(provider.RoleUser, provider.RoleAssistant)
	if leaf := b.app.Agent.Session().Leaf(); leaf != "" && (len(refs) == 0 || refs[0].ID != leaf) {
		refs = append([]app.MessageRef{{ID: leaf, Text: "Current position (everything so far)"}}, refs...)
	}
	return refs
}

// Fork copies the conversation up to entry id into a new session and
// switches to it. It returns the new session's path.
func (b *Bridge) Fork(id string) (string, error) {
	s, err := b.app.Fork(id)
	if err != nil {
		return "", err
	}
	if err := b.app.Agent.SetSession(s); err != nil {
		return "", err
	}
	return s.Path(), nil
}

// Tree lists the session tree.
func (b *Bridge) Tree() []app.TreeNode { return b.app.Tree() }

// Checkout moves to another entry of the session tree.
func (b *Bridge) Checkout(id string) error { return b.app.Agent.Checkout(id) }

// Label sets (or with an empty label removes) the label of entry id; an
// empty id labels the current position.
func (b *Bridge) Label(id, label string) error {
	if id == "" {
		id = b.app.Agent.Session().Leaf()
		if id == "" {
			return errors.New("nothing to label yet")
		}
	}
	return b.app.Agent.Label(id, label)
}

// StartAside starts a read-only side conversation.
func (b *Bridge) StartAside() error { return b.app.Agent.StartAside() }

// EndAside returns from a side conversation, carrying an optional note.
func (b *Bridge) EndAside(note string) error { return b.app.Agent.EndAside(note) }

// SummarizeAside drafts a note summarizing the side conversation.
func (b *Bridge) SummarizeAside(ctx context.Context) (string, error) {
	return b.app.Agent.SummarizeAside(ctx)
}

// Compact summarizes older messages to free context.
func (b *Bridge) Compact(ctx context.Context) error { return b.app.Agent.Compact(ctx) }

// CompactionSummary returns the summary stored in a compaction entry.
func (b *Bridge) CompactionSummary(entryID string) string {
	e, _ := b.app.Agent.Session().Get(entryID)
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
	r, err := b.app.Agent.ContextReport()
	if err != nil {
		return ContextInfo{}, err
	}
	info := ContextInfo{
		Tools: len(b.app.Agent.Tools()), ToolTokens: r.ToolTokens,
		Messages: r.Messages, MessageTokens: r.MessageTokens,
		Total: r.Total, Window: r.Window, Compacted: r.Compacted,
	}
	for _, p := range r.Parts {
		info.Parts = append(info.Parts, ContextPart{Name: p.Name, Path: p.Path, Tokens: p.Tokens})
	}
	for _, s := range b.app.Skills {
		info.Skills = append(info.Skills, s.Name)
	}
	return info, nil
}

// Copy copies the last answer (or with all the whole conversation) to the
// clipboard. It reports false when there is nothing to copy.
func (b *Bridge) Copy(all bool) (bool, error) {
	text := b.app.CopyText(all)
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
	cfg, model := b.app.Agent.Config(), b.app.Agent.Model().ID
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
	saved, note, err := b.app.SetSetting(key, value)
	if err != nil {
		return SettingResult{}, err
	}
	s, _ := config.LookupSetting(key)
	return SettingResult{Saved: saved, Note: note, Value: s.Value(b.app.Agent.Config(), b.app.Agent.Model().ID)}, nil
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
	if b.app.Ext == nil {
		return nil
	}
	var out []ExtensionInfo
	for _, x := range b.app.Ext.Extensions() {
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
	return b.app.SetExtensionEnabled(ctx, id, on)
}

// Reload reloads skills, prompt templates and context files and restarts
// the extensions.
func (b *Bridge) Reload(ctx context.Context) error {
	skills, w1 := prompt.LoadSkills(b.app.Paths)
	templates, w2 := prompt.LoadTemplates(b.app.Paths)
	b.app.Skills = skills
	b.app.Templates = templates
	b.app.Agent.SetSkills(skills)
	for _, w := range slices.Concat(w1, w2) {
		b.push(events.Notice{Level: "warn", Text: w})
	}
	if err := b.app.ReloadExtensions(ctx); err != nil {
		return err
	}
	what := "skills, prompt templates and context files"
	if b.app.Ext != nil {
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
