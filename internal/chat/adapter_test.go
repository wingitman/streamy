package chat

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type contractAdapter struct {
	id     ConnectionID
	status ConnectionStatus
	events chan Event
}

var _ Adapter = (*contractAdapter)(nil)

func (a *contractAdapter) ConnectionID() ConnectionID { return a.id }
func (a *contractAdapter) Platform() Platform         { return a.status.Platform }
func (a *contractAdapter) Capabilities() Capabilities {
	return Capabilities{ReceiveChat: true, SendChat: true, CursorResume: true}
}
func (a *contractAdapter) Status() ConnectionStatus { return a.status }
func (a *contractAdapter) Events() <-chan Event     { return a.events }

func (a *contractAdapter) Connect(context.Context) error {
	a.status.State = StateConnected
	a.events <- StatusEvent{Status: a.status}
	return nil
}

func (a *contractAdapter) Disconnect(context.Context) error {
	a.status.State = StateDisconnected
	close(a.events)
	return nil
}

func (a *contractAdapter) Send(_ context.Context, request SendRequest) SendResult {
	return SendResult{
		LocalID:      request.LocalID,
		ConnectionID: a.id,
		Platform:     a.status.Platform,
		Status:       DeliverySent,
		ProviderID:   "provider-message-1",
	}
}

func TestAdapterLifecycleEmitsStatusAndClosesEvents(t *testing.T) {
	adapter := &contractAdapter{
		id: "twitch-main",
		status: ConnectionStatus{
			ConnectionID: "twitch-main",
			Platform:     PlatformTwitch,
			State:        StateDisconnected,
			At:           time.Now(),
		},
		events: make(chan Event, 1),
	}

	if err := adapter.Connect(context.Background()); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	if got := adapter.Status().State; got != StateConnected {
		t.Fatalf("state = %q, want %q", got, StateConnected)
	}
	if _, ok := (<-adapter.Events()).(StatusEvent); !ok {
		t.Fatal("Connect() did not emit a StatusEvent")
	}
	if err := adapter.Disconnect(context.Background()); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if _, ok := <-adapter.Events(); ok {
		t.Fatal("Events() remained open after Disconnect()")
	}
}

func TestAdapterSendReturnsPerConnectionResult(t *testing.T) {
	adapter := &contractAdapter{
		id:     "youtube-main",
		status: ConnectionStatus{Platform: PlatformYouTube},
		events: make(chan Event),
	}

	result := adapter.Send(context.Background(), SendRequest{LocalID: "local-1", Text: "hello"})
	if result.Status != DeliverySent {
		t.Fatalf("status = %q, want %q", result.Status, DeliverySent)
	}
	if result.LocalID != "local-1" || result.ConnectionID != "youtube-main" || result.Platform != PlatformYouTube {
		t.Fatalf("result identity = %#v", result)
	}
	if result.ProviderID == "" {
		t.Fatal("sent result has no provider ID")
	}
}

func TestMessageCarriesNormalizedPriorityAndOptionalEvents(t *testing.T) {
	visibleUntil := time.Now().Add(30 * time.Second)
	message := Message{
		LocalID:      "local-1",
		ProviderID:   "twitch-message-1",
		Platform:     PlatformTwitch,
		Kind:         MessageKindPaid,
		Status:       MessageReceived,
		Priority:     PriorityPaid,
		Highlight:    true,
		VisibleUntil: visibleUntil,
		Paid: &PaidEvent{
			AmountMinor:   500,
			Currency:      "USD",
			DisplayAmount: "$5.00",
		},
		ProviderMetadata: map[string]any{"reward_id": "provider-reward-1"},
	}

	if message.Kind != MessageKindPaid || message.Status != MessageReceived {
		t.Fatalf("message classification = %#v", message)
	}
	if message.Priority != PriorityPaid || !message.Highlight {
		t.Fatalf("message priority = %#v", message)
	}
	if !message.VisibleUntil.After(time.Now()) {
		t.Fatal("message visibility deadline was not retained")
	}
	if message.Paid == nil || message.Paid.AmountMinor != 500 {
		t.Fatalf("paid event = %#v", message.Paid)
	}
	if message.ProviderMetadata["reward_id"] != "provider-reward-1" {
		t.Fatalf("provider metadata = %#v", message.ProviderMetadata)
	}
}

func TestMessageUpdatePreservesProviderIdentity(t *testing.T) {
	update := MessageUpdateEvent{
		ProviderID: "youtube-message-1",
		Message: Message{
			ProviderID: "youtube-message-1",
			Status:     MessageUpdated,
			Text:       "edited text",
		},
	}

	if update.ProviderID != update.Message.ProviderID {
		t.Fatalf("update identity mismatch: %#v", update)
	}
	if update.Message.Status != MessageUpdated {
		t.Fatalf("update status = %q, want %q", update.Message.Status, MessageUpdated)
	}
}

func TestBurstControllerEnforcesRollingRenderBudget(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	controller, err := NewBurstController(BurstConfig{MaxMessages: 2, Window: 5 * time.Second}, nil, clock)
	if err != nil {
		t.Fatalf("NewBurstController() error = %v", err)
	}

	if !controller.Offer(Message{LocalID: "1"}).RenderNow {
		t.Fatal("first message was suppressed")
	}
	if !controller.Offer(Message{LocalID: "2"}).RenderNow {
		t.Fatal("second message was suppressed")
	}
	suppressed := controller.Offer(Message{LocalID: "3"})
	if suppressed.RenderNow || !suppressed.Suppressed {
		t.Fatalf("third decision = %#v, want suppressed", suppressed)
	}

	now = now.Add(5 * time.Second)
	if !controller.Offer(Message{LocalID: "4"}).RenderNow {
		t.Fatal("message was not admitted after the rolling window elapsed")
	}
}

func TestBurstControllerDecoratesPriorityWithoutReordering(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	controller, err := NewBurstController(DefaultBurstConfig(), nil, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewBurstController() error = %v", err)
	}

	message := Message{LocalID: "paid-1", Priority: PriorityPaid}
	decision := controller.Offer(message)
	if decision.Message.LocalID != "paid-1" || !decision.RenderNow {
		t.Fatalf("decision message = %#v", decision)
	}
	if decision.PriorityWeight != 5 || !decision.Highlight {
		t.Fatalf("priority decoration = %#v", decision)
	}
	if got := decision.VisibleUntil.Sub(now); got != 25*time.Second {
		t.Fatalf("visible duration = %s, want 25s", got)
	}
}

func TestBurstControllerRejectsInvalidConfig(t *testing.T) {
	if _, err := NewBurstController(BurstConfig{}, nil, nil); err == nil {
		t.Fatal("invalid burst config was accepted")
	}
}

func TestHistoryRetainsBoundedChronologicalPage(t *testing.T) {
	history, err := NewHistory(testHistoryConfig(t, 3, 1024*1024))
	if err != nil {
		t.Fatalf("NewHistory() error = %v", err)
	}
	defer history.Close()

	for i := 1; i <= 4; i++ {
		if err := history.Append(Message{LocalID: string(rune('0' + i)), Text: "message"}); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}
	if got := history.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}
	page := history.Page(0, 2)
	if len(page) != 2 || page[0].LocalID != "2" || page[1].LocalID != "3" {
		t.Fatalf("Page() = %#v", page)
	}
}

func TestHistoryUsesConfiguredChatMessagePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "streamy-history.toml")
	config := testHistoryConfig(t, 10, 1024*1024)
	config.Path = path
	history, err := NewHistory(config)
	if err != nil {
		t.Fatalf("NewHistory() error = %v", err)
	}
	if err := history.Append(Message{ProviderID: "message-1", Text: "hello"}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := history.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("configured history path was not created: %v", err)
	}
}

func TestHistoryRotatesAtConfiguredLogSize(t *testing.T) {
	config := testHistoryConfig(t, 100, 700)
	history, err := NewHistory(config)
	if err != nil {
		t.Fatalf("NewHistory() error = %v", err)
	}
	text := strings.Repeat("x", 300)
	if err := history.Append(Message{LocalID: "1", Text: text}); err != nil {
		t.Fatalf("first Append() error = %v", err)
	}
	if err := history.Append(Message{LocalID: "2", Text: text}); err != nil {
		t.Fatalf("second Append() error = %v", err)
	}
	if err := history.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	entries, err := os.ReadDir(config.Directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("history files = %d, want active and rotated files", len(entries))
	}
}

func TestHistoryRecoversAroundCorruptLinesAndAppliesPrivacy(t *testing.T) {
	config := testHistoryConfig(t, 10, 1024*1024)
	config.PersistText = false
	config.PersistAuthor = false
	config.PersistProviderData = false
	path := filepath.Join(config.Directory, "session-twitch-main.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	history, err := NewHistory(config)
	if err != nil {
		t.Fatalf("NewHistory() error = %v", err)
	}
	defer history.Close()
	if err := history.Append(Message{LocalID: "1", Text: "private", Author: Author{Name: "person"}, ProviderMetadata: map[string]any{"token": "secret"}}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if err := history.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	reopened, err := NewHistory(config)
	if err != nil {
		t.Fatalf("reopen history: %v", err)
	}
	defer reopened.Close()
	if got := reopened.Page(0, 1)[0]; got.Text != "" || got.Author.Name != "" || got.ProviderMetadata != nil {
		t.Fatalf("privacy settings were not applied: %#v", got)
	}
}

func TestHistoryConcurrentAppendsStayBounded(t *testing.T) {
	history, err := NewHistory(testHistoryConfig(t, 100, 1024*1024))
	if err != nil {
		t.Fatalf("NewHistory() error = %v", err)
	}
	defer history.Close()

	var wait sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for message := 0; message < 25; message++ {
				_ = history.Append(Message{LocalID: fmt.Sprintf("%d-%d", worker, message)})
			}
		}(worker)
	}
	wait.Wait()
	if got := history.Len(); got != 100 {
		t.Fatalf("Len() = %d, want bounded 100", got)
	}
}

func testHistoryConfig(t *testing.T, maxMessages int, maxBytes int64) HistoryConfig {
	t.Helper()
	return HistoryConfig{
		Directory:     t.TempDir(),
		SessionID:     "session",
		ConnectionID:  "main",
		Platform:      PlatformTwitch,
		MaxMessages:   maxMessages,
		MaxBytes:      maxBytes,
		PersistText:   true,
		PersistAuthor: true,
	}
}
