package youtube

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wingitman/streamy/internal/chat"
)

func TestNormalizeSuperChatAndMembership(t *testing.T) {
	adapter, err := New(Config{ConnectionID: "main", LiveChatID: "chat-1", ClientID: "client", AccessToken: "token"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	item := youtubeItem{ID: "message-1"}
	item.Snippet.DisplayMessage = "Thanks!"
	item.Snippet.Type = "superChatEvent"
	item.Snippet.SuperChat.AmountMicros = 5000000
	item.Snippet.SuperChat.Currency = "USD"
	item.Snippet.SuperChat.AmountDisplayString = "$5.00"
	item.Author.ID, item.Author.Name = "user-1", "Alice"
	item.Author.Moderator = true

	message, ok := adapter.normalize(item)
	if !ok || message.Kind != chat.MessageKindPaid || message.Paid == nil || message.Paid.AmountMinor != 5000000 {
		t.Fatalf("normalized super chat = %#v, ok=%v", message, ok)
	}
	if !message.Author.IsModerator || message.Author.DisplayName != "Alice" {
		t.Fatalf("author = %#v", message.Author)
	}

	item.ID = "membership-1"
	item.Snippet.Type = "memberMilestoneChatEvent"
	item.Snippet.SuperChat.AmountMicros = 0
	item.Snippet.Membership.MemberMonths = 12
	message, ok = adapter.normalize(item)
	if !ok || message.Kind != chat.MessageKindMembership || message.Membership == nil || message.Membership.Months != 12 {
		t.Fatalf("normalized membership = %#v, ok=%v", message, ok)
	}
}

func TestSendLiveChatMessageUsesOAuthBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/liveChat/messages" || request.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("request = %s %s headers=%v", request.Method, request.URL, request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), `"messageText":"hello"`) || !strings.Contains(string(body), `"liveChatId":"chat-1"`) {
			t.Errorf("request body = %s", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"id":"provider-1"}`)
	}))
	defer server.Close()
	adapter, err := New(Config{ConnectionID: "main", LiveChatID: "chat-1", ClientID: "client", AccessToken: "token", APIURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result := adapter.Send(context.Background(), chat.SendRequest{LocalID: "local-1", Text: "hello"})
	if result.Status != chat.DeliverySent || result.ProviderID != "provider-1" {
		t.Fatalf("send result = %#v", result)
	}
}

func TestSendRejectsOversizedMessageBeforeRequest(t *testing.T) {
	adapter, err := New(Config{ConnectionID: "main", LiveChatID: "chat", ClientID: "client", AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	result := adapter.Send(context.Background(), chat.SendRequest{LocalID: "local-1", Text: strings.Repeat("界", 201)})
	if result.Status != chat.DeliveryDropped || result.DropReason != "message is 201/200 characters" {
		t.Fatalf("result = %#v", result)
	}
}

func TestReceiveStreamDecodesPagesAndResumesCursor(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		if requests == 1 {
			if got := request.URL.Query().Get("pageToken"); got != "" {
				t.Errorf("initial page token = %q", got)
			}
			_, _ = io.WriteString(writer, `{"nextPageToken":"cursor-1","items":[{"id":"message-1","snippet":{"displayMessage":"hello"},"authorDetails":{"displayName":"Alice"}}]}`)
			return
		}
		if got := request.URL.Query().Get("pageToken"); got != "cursor-1" {
			t.Errorf("resumed page token = %q", got)
		}
		_, _ = io.WriteString(writer, `{"items":[{"id":"message-2","snippet":{"displayMessage":"again"},"authorDetails":{"displayName":"Bob"}}]}`)
	}))
	defer server.Close()

	adapter, err := New(Config{ConnectionID: "main", LiveChatID: "chat-1", ClientID: "client", AccessToken: "token", APIURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := adapter.receiveStream(context.Background()); err != io.EOF {
		t.Fatalf("receiveStream() error = %v", err)
	}
	if err := adapter.receiveStream(context.Background()); err != io.EOF {
		t.Fatalf("resumed receiveStream() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d", requests)
	}
	var messages []chat.Message
	for len(adapter.events) > 0 {
		event := <-adapter.events
		switch event := event.(type) {
		case chat.MessageEvent:
			messages = append(messages, event.Message)
		}
	}
	if len(messages) != 2 || messages[0].Text != "hello" || messages[1].Text != "again" {
		t.Fatalf("messages = %#v", messages)
	}
}

func TestAdapterCanReconnectWithoutClosingEvents(t *testing.T) {
	adapter, err := New(Config{ConnectionID: "main", LiveChatID: "chat-1", ClientID: "client", AccessToken: "token", APIURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for range 2 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := adapter.Connect(ctx); err != nil {
			t.Fatalf("Connect() error = %v", err)
		}
		done := adapter.done
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("adapter did not stop")
		}
	}
	select {
	case _, ok := <-adapter.Events():
		if !ok {
			t.Fatal("Events() closed after disconnect")
		}
	default:
	}
}
