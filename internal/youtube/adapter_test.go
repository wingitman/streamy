package youtube

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
