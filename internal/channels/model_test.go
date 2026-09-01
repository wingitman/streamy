package channels

import (
	"context"
	"testing"

	"github.com/wingitman/streamy/internal/chat"
)

type fakeAdapter struct {
	id        chat.ConnectionID
	platform  chat.Platform
	sendCalls int
}

var _ chat.Adapter = (*fakeAdapter)(nil)

func (a *fakeAdapter) ConnectionID() chat.ConnectionID { return a.id }
func (a *fakeAdapter) Platform() chat.Platform         { return a.platform }
func (a *fakeAdapter) Capabilities() chat.Capabilities { return chat.Capabilities{SendChat: true} }
func (a *fakeAdapter) Status() chat.ConnectionStatus {
	return chat.ConnectionStatus{ConnectionID: a.id, Platform: a.platform, State: chat.StateConnected}
}
func (a *fakeAdapter) Events() <-chan chat.Event        { return make(chan chat.Event) }
func (a *fakeAdapter) Connect(context.Context) error    { return nil }
func (a *fakeAdapter) Disconnect(context.Context) error { return nil }
func (a *fakeAdapter) Send(_ context.Context, request chat.SendRequest) chat.SendResult {
	a.sendCalls++
	return chat.SendResult{LocalID: request.LocalID, ConnectionID: a.id, Platform: a.platform, Status: chat.DeliverySent}
}

func TestModelFiltersByViewAndText(t *testing.T) {
	model := NewModel([]chat.Adapter{
		&fakeAdapter{id: "tw", platform: chat.PlatformTwitch},
		&fakeAdapter{id: "yt", platform: chat.PlatformYouTube},
	})
	model.AddMessage(chat.RenderDecision{RenderNow: true, Message: chat.Message{Platform: chat.PlatformTwitch, Text: "Hello Twitch", Author: chat.Author{DisplayName: "Alice"}}})
	model.AddMessage(chat.RenderDecision{RenderNow: true, Message: chat.Message{Platform: chat.PlatformYouTube, Text: "Hello YouTube", Author: chat.Author{DisplayName: "Bob"}}})

	if got := len(model.Messages()); got != 2 {
		t.Fatalf("combined messages = %d, want 2", got)
	}
	if err := model.SetView(ViewTwitch); err != nil {
		t.Fatalf("SetView() error = %v", err)
	}
	model.SetFilter("alice")
	messages := model.Messages()
	if len(messages) != 1 || messages[0].Text != "Hello Twitch" {
		t.Fatalf("filtered Twitch messages = %#v", messages)
	}
}

func TestModelSubmitAllCreatesIndependentPendingDeliveries(t *testing.T) {
	twitch := &fakeAdapter{id: "tw", platform: chat.PlatformTwitch}
	youtube := &fakeAdapter{id: "yt", platform: chat.PlatformYouTube}
	model := NewModel([]chat.Adapter{twitch, youtube})
	model.SetComposer("  hello both  ")
	commands, err := model.Submit()
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if len(commands) != 2 || model.Composer() != "" {
		t.Fatalf("submit commands/composer = %d/%q", len(commands), model.Composer())
	}
	if twitch.sendCalls != 0 || youtube.sendCalls != 0 {
		t.Fatal("Submit() performed blocking adapter work")
	}
	for _, command := range commands {
		if err := model.ApplyResult(command.Adapter.Send(context.Background(), command.Request)); err != nil {
			t.Fatalf("ApplyResult() error = %v", err)
		}
	}
	for _, delivery := range model.Deliveries() {
		if delivery.State != DeliverySent || delivery.Attempts != 1 {
			t.Fatalf("delivery = %#v", delivery)
		}
	}
}

func TestModelRetryOnlyRetriesRetryableFailure(t *testing.T) {
	adapter := &fakeAdapter{id: "tw", platform: chat.PlatformTwitch}
	model := NewModel([]chat.Adapter{adapter})
	model.SetComposer("hello")
	commands, err := model.Submit()
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	failed := chat.SendResult{LocalID: commands[0].Request.LocalID, ConnectionID: adapter.id, Platform: adapter.platform, Status: chat.DeliveryFailed, Retryable: true}
	if err := model.ApplyResult(failed); err != nil {
		t.Fatalf("ApplyResult() error = %v", err)
	}
	retry, err := model.Retry(failed.LocalID, adapter.id)
	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if retry.Request.Text != "hello" || model.Deliveries()[0].State != DeliveryRetrying {
		t.Fatalf("retry = %#v, deliveries = %#v", retry, model.Deliveries())
	}
}

func TestModelSuppressedPageIsSeparateFromLiveMessages(t *testing.T) {
	model := NewModel(nil)
	model.AddMessage(chat.RenderDecision{RenderNow: true, Message: chat.Message{Text: "live"}})
	model.AddMessage(chat.RenderDecision{Suppressed: true, Message: chat.Message{Text: "backlog"}})

	if got := model.Messages(); len(got) != 1 || got[0].Text != "live" {
		t.Fatalf("live messages = %#v", got)
	}
	if got := model.SuppressedPage(0, 10); len(got) != 1 || got[0].Text != "backlog" {
		t.Fatalf("suppressed page = %#v", got)
	}
}
