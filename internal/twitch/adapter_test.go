package twitch

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

func TestNormalizeChatEventPreservesIdentityAndPaidData(t *testing.T) {
	adapter, err := New(Config{ConnectionID: "main", Channel: "streamer", BroadcasterID: "100", UserID: "200", ClientID: "client", AccessToken: "token"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	event := chatEvent{MessageID: "message-1", ChatterUserID: "user-1", ChatterUserName: "alice", Message: struct {
		Text      string `json:"text"`
		Fragments []struct {
			Type  string `json:"type"`
			Cheer struct {
				Bits int64 `json:"bits"`
			} `json:"cheer"`
		} `json:"fragments"`
	}{Text: "hello"}}
	event.Message.Fragments = append(event.Message.Fragments, struct {
		Type  string `json:"type"`
		Cheer struct {
			Bits int64 `json:"bits"`
		} `json:"cheer"`
	}{Type: "cheermote", Cheer: struct {
		Bits int64 `json:"bits"`
	}{Bits: 100}})

	message, ok := adapter.normalize(event, "envelope-1")
	if !ok || message.ProviderID != "message-1" || message.Author.DisplayName != "alice" {
		t.Fatalf("normalized message = %#v, ok=%v", message, ok)
	}
	if message.Kind != chat.MessageKindPaid || message.Paid == nil || message.Paid.AmountMinor != 100 {
		t.Fatalf("paid message = %#v", message)
	}
	if _, ok := adapter.normalize(event, "envelope-2"); ok {
		t.Fatal("duplicate event was emitted")
	}
}

func TestSendChatMessageUsesHelixHeadersAndResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/messages" || request.Header.Get("Authorization") != "Bearer token" || request.Header.Get("Client-Id") != "client" {
			t.Errorf("request = %s %s headers=%v", request.Method, request.URL, request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), `"message":"hello"`) {
			t.Errorf("request body = %s", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"data":[{"message_id":"provider-1","is_sent":true}]}`)
	}))
	defer server.Close()
	adapter, err := New(Config{ConnectionID: "main", Channel: "streamer", BroadcasterID: "100", UserID: "200", ClientID: "client", AccessToken: "token", APIURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	result := adapter.Send(context.Background(), chat.SendRequest{LocalID: "local-1", Text: "hello"})
	if result.Status != chat.DeliverySent || result.ProviderID != "provider-1" {
		t.Fatalf("send result = %#v", result)
	}
}

func TestSendRejectsOversizedMessageBeforeRequest(t *testing.T) {
	adapter, err := New(Config{ConnectionID: "main", Channel: "channel", BroadcasterID: "broadcaster", UserID: "user", ClientID: "client", AccessToken: "token"})
	if err != nil {
		t.Fatal(err)
	}
	result := adapter.Send(context.Background(), chat.SendRequest{LocalID: "local-1", Text: strings.Repeat("x", 501)})
	if result.Status != chat.DeliveryDropped || result.DropReason != "message is 501/500 characters" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSubscribeUsesEventSubWebsocketPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/eventsub/subscriptions" || request.Header.Get("Authorization") != "Bearer token" || request.Header.Get("Client-Id") != "client" {
			t.Errorf("request = %s %s headers=%v", request.Method, request.URL, request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		payload := string(body)
		for _, expected := range []string{`"type":"channel.chat.message"`, `"broadcaster_user_id":"100"`, `"user_id":"200"`, `"session_id":"session-1"`} {
			if !strings.Contains(payload, expected) {
				t.Errorf("subscription body %q does not contain %q", payload, expected)
			}
		}
		writer.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	adapter, err := New(Config{ConnectionID: "main", Channel: "streamer", BroadcasterID: "100", UserID: "200", ClientID: "client", AccessToken: "token", APIURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := adapter.subscribe(context.Background(), "session-1"); err != nil {
		t.Fatalf("subscribe() error = %v", err)
	}
}

func TestAdapterCanReconnectWithoutClosingEvents(t *testing.T) {
	adapter, err := New(Config{ConnectionID: "main", Channel: "streamer", BroadcasterID: "100", UserID: "200", ClientID: "client", AccessToken: "token", EventSubURL: "ws://127.0.0.1:1"})
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
