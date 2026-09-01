package app

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/wingitman/streamy/internal/auth"
	"github.com/wingitman/streamy/internal/channels"
	"github.com/wingitman/streamy/internal/chat"
	"github.com/wingitman/streamy/internal/config"
)

func TestEmptyChatUsesGeneratedBaselineLayout(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := updated.(Model).View().Content
	if !strings.Contains(view, "streamy") || !strings.Contains(view, "delby") || !strings.Contains(view, "soft") {
		t.Fatalf("baseline view = %q", view)
	}
}

func TestWarningsRemainVisibleInChatView(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.SetWarnings([]string{"twitch-main unavailable: missing token"})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	if !strings.Contains(updated.(Model).View().Content, "twitch-main unavailable: missing token") {
		t.Fatal("startup warning was not rendered")
	}
}

func TestMessageLinesWrapToWindowWidth(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.width = 32
	model.channels = channels.NewModel(nil)
	model.channels.AddMessage(chat.RenderDecision{RenderNow: true, Message: chat.Message{
		ConnectionID: "tw", ProviderID: "message-1", Platform: chat.PlatformTwitch,
		Author: chat.Author{DisplayName: "Alice"}, Text: "one two three four five six seven eight nine ten",
	}})
	lines := model.messageLines()
	if len(lines) < 2 {
		t.Fatalf("message was not wrapped: %#v", lines)
	}
	for _, line := range lines {
		if len([]rune(line)) > model.width {
			t.Fatalf("line exceeds width: %q", line)
		}
	}
}

func TestChatPasteFillsComposer(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := model.Update(tea.PasteMsg{Content: "hello\nworld"})
	if got, want := updated.(Model).composer, "hello world"; got != want {
		t.Fatalf("pasted composer = %q, want %q", got, want)
	}
}

func TestMessageLengthStatusUsesSelectedProviderLimit(t *testing.T) {
	adapter := &testAppAdapter{id: "twitch-main", platform: chat.PlatformTwitch, maxLength: 500}
	model, err := New("streamy", config.Default(), []chat.Adapter{adapter}, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.composer = "hello"
	if got, want := model.messageLengthStatus(), "message: 5/500 characters"; got != want {
		t.Fatalf("messageLengthStatus() = %q, want %q", got, want)
	}
}

func TestLargePasteKeepsFooterVisible(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.width, model.height = 80, 12
	model.composer = strings.Repeat("long pasted text ", 100)
	view := model.View().Content
	if !strings.Contains(view, "[esc]") {
		t.Fatalf("footer hints were clipped from view: %q", view)
	}
	if !strings.Contains(view, "...") {
		t.Fatal("large composer was not visibly clipped")
	}
	if got := model.composer; got == "" || !strings.HasPrefix(got, "long pasted text") {
		t.Fatal("full composer draft was not retained")
	}
}

func TestComposerSupportsCursorMovementAndMiddleInsertion(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.composer = "hello world"
	model.composerCursor = len([]rune(model.composer))
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl})
	model = updated.(Model)
	if model.composerCursor != 6 {
		t.Fatalf("Ctrl+Left cursor = %d, want 6", model.composerCursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: "X", Code: 'X'})
	model = updated.(Model)
	if model.composer != "hello Xworld" || model.composerCursor != 7 {
		t.Fatalf("middle insertion = %q, cursor %d", model.composer, model.composerCursor)
	}
}

func TestComposerWordDeletionIsRuneSafe(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.composer = "hello 世界"
	model.composerCursor = len([]rune(model.composer))
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModCtrl})
	model = updated.(Model)
	if model.composer != "hello " || model.composerCursor != 6 {
		t.Fatalf("Ctrl+Backspace = %q, cursor %d", model.composer, model.composerCursor)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyDelete, Mod: tea.ModCtrl})
	model = updated.(Model)
	if model.composer != "hello " || model.composerCursor != 6 {
		t.Fatalf("Ctrl+Delete at end changed composer = %q, cursor %d", model.composer, model.composerCursor)
	}
}

func TestComposerDoesNotInsertSpecialKeyNames(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, code := range []rune{tea.KeyTab, tea.KeyDelete} {
		updated, _ := model.Update(tea.KeyPressMsg{Code: code})
		model = updated.(Model)
	}
	if model.composer != "" {
		t.Fatalf("special keys became composer text: %q", model.composer)
	}
}

func TestComposerViewExposesCursor(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.composer = "hello"
	model.composerCursor = 2
	model.width, model.height = 80, 24
	if view := model.View(); view.Cursor == nil {
		t.Fatal("composer cursor was not exposed in the view")
	}
}

func TestComposerIsRenderedOnceButCopyKeepsFullDraft(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.width, model.height = 80, 24
	model.composer = strings.Repeat("filler ", 20) + "unique-marker"
	view := model.View().Content
	if got := strings.Count(view, "unique-marker"); got != 1 {
		t.Fatalf("composer rendered %d times, want once", got)
	}
	if got := model.commandPreview(); got != model.composer {
		t.Fatal("copy preview no longer contains the full draft")
	}
}

type testAppAdapter struct {
	id        chat.ConnectionID
	platform  chat.Platform
	maxLength int
}

func (a *testAppAdapter) ConnectionID() chat.ConnectionID { return a.id }
func (a *testAppAdapter) Platform() chat.Platform         { return a.platform }
func (a *testAppAdapter) Capabilities() chat.Capabilities {
	return chat.Capabilities{SendChat: true, MaxMessageLength: a.maxLength}
}
func (a *testAppAdapter) Status() chat.ConnectionStatus {
	return chat.ConnectionStatus{ConnectionID: a.id, Platform: a.platform}
}
func (a *testAppAdapter) Events() <-chan chat.Event        { return make(chan chat.Event) }
func (a *testAppAdapter) Connect(context.Context) error    { return nil }
func (a *testAppAdapter) Disconnect(context.Context) error { return nil }
func (a *testAppAdapter) Send(context.Context, chat.SendRequest) chat.SendResult {
	return chat.SendResult{}
}

func TestMaskCredentialKeepsOnlySmallPrefixAndSuffix(t *testing.T) {
	if got, want := maskCredential("12345678901234567890"), "12****************90"; got != want {
		t.Fatalf("maskCredential() = %q, want %q", got, want)
	}
	if got, want := maskCredential("abcd"), "****"; got != want {
		t.Fatalf("short maskCredential() = %q, want %q", got, want)
	}
}

func TestNextConnectionIDAvoidsExistingNames(t *testing.T) {
	cfg := config.Config{Config: auth.Config{Connections: []auth.ConnectionConfig{
		{ID: "twitch-main", Platform: chat.PlatformTwitch},
		{ID: "twitch-main-2", Platform: chat.PlatformTwitch},
	}}}
	if got, want := nextConnectionID(cfg, chat.PlatformTwitch), "twitch-main-3"; got != want {
		t.Fatalf("nextConnectionID() = %q, want %q", got, want)
	}
}

func TestSetupPasteOnlyAppliesWhileEditing(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.current = modeIntegrationSetup
	model.setupField = 0
	updated, _ := model.Update(tea.PasteMsg{Content: "ignored"})
	if got := updated.(Model).setupValues[0]; got != "" {
		t.Fatalf("paste while inactive changed field to %q", got)
	}
	model.setupEditing = true
	updated, _ = model.Update(tea.PasteMsg{Content: "accepted\n"})
	if got, want := updated.(Model).setupValues[0], "accepted"; got != want {
		t.Fatalf("paste while editing = %q, want %q", got, want)
	}
}

func TestSetupTabMovesFromSecretToSave(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.current = modeIntegrationSetup
	model.setupField = 3
	model.cfg.Keybinds.NextTarget = "tab"
	updated, _ := model.integrationSetupKey(model.cfg.Keybinds.NextTarget)
	if got, want := updated.(Model).setupField, 4; got != want {
		t.Fatalf("setup field after final Tab = %d, want %d", got, want)
	}
}

func TestQuitKeyIsEscapeAndQCanBeTyped(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := model.key("q")
	if cmd != nil || updated.(Model).composer != "q" {
		t.Fatalf("q should be typed, composer = %q", updated.(Model).composer)
	}
	_, cmd = model.key("esc")
	if cmd == nil {
		t.Fatal("escape should quit")
	}
}

func TestUnconfiguredOpenConfigKeyIsTyped(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := model.key("o")
	if cmd != nil || updated.(Model).composer != "o" {
		t.Fatalf("o should be typed when open_config is %q, composer = %q", config.Default().Keybinds.OpenConfig, updated.(Model).composer)
	}
}

func TestRetainedSecretIsReplacedOnFirstEdit(t *testing.T) {
	model, err := New("streamy", config.Default(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	model.current = modeIntegrationSetup
	model.setupField = 3
	model.setupEditing = true
	model.setupSecretKept = true
	model.setupValues[3] = "previous-secret"
	updated, _ := model.integrationSetupKey("n")
	if got := updated.(Model).setupValues[3]; got != "n" {
		t.Fatalf("replaced secret = %q, want %q", got, "n")
	}
}
