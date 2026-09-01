package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/wingitman/streamy/internal/chat"
)

const (
	defaultAPIURL      = "https://api.twitch.tv/helix"
	defaultEventSubURL = "wss://eventsub.wss.twitch.tv/ws"
)

type Config struct {
	ConnectionID  chat.ConnectionID
	Channel       string
	BroadcasterID string
	UserID        string
	ClientID      string
	AccessToken   string
	APIURL        string
	EventSubURL   string
	HTTPClient    *http.Client
}

type Adapter struct {
	config Config
	client *http.Client

	mu     sync.RWMutex
	status chat.ConnectionStatus
	cancel context.CancelFunc
	done   chan struct{}
	events chan chat.Event

	seenMu  sync.Mutex
	seen    map[string]struct{}
	seenIDs []string
}

func New(config Config) (*Adapter, error) {
	if config.ConnectionID == "" || config.Channel == "" || config.BroadcasterID == "" || config.UserID == "" || config.ClientID == "" || config.AccessToken == "" {
		return nil, errors.New("Twitch connection, channel, user, client, and token settings are required")
	}
	if config.APIURL == "" {
		config.APIURL = defaultAPIURL
	}
	if config.EventSubURL == "" {
		config.EventSubURL = defaultEventSubURL
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Adapter{
		config: config,
		client: config.HTTPClient,
		status: chat.ConnectionStatus{ConnectionID: config.ConnectionID, Platform: chat.PlatformTwitch, State: chat.StateDisconnected},
		events: make(chan chat.Event, 256),
		seen:   make(map[string]struct{}),
	}, nil
}

func (a *Adapter) ConnectionID() chat.ConnectionID { return a.config.ConnectionID }
func (a *Adapter) Platform() chat.Platform         { return chat.PlatformTwitch }
func (a *Adapter) Capabilities() chat.Capabilities {
	return chat.Capabilities{
		ReceiveChat: true, SendChat: true, MessageBadges: true, MessageEmotes: true,
		PaidMessages: true, MembershipMessages: false, FollowerState: false, FirstTimeState: false,
		CursorResume: false,
	}
}
func (a *Adapter) Status() chat.ConnectionStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}
func (a *Adapter) Events() <-chan chat.Event { return a.events }

func (a *Adapter) Connect(parent context.Context) error {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return errors.New("Twitch adapter is already connected")
	}
	ctx, cancel := context.WithCancel(parent)
	a.cancel = cancel
	a.done = make(chan struct{})
	a.setStatusLocked(chat.StateConnecting, "connecting", true)
	done := a.done
	a.mu.Unlock()
	go func() {
		defer close(done)
		a.run(ctx)
	}()
	return nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	cancel := a.cancel
	done := a.done
	if cancel == nil {
		a.mu.Unlock()
		return nil
	}
	a.setStatusLocked(chat.StateStopping, "stopping", false)
	a.mu.Unlock()
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *Adapter) Send(ctx context.Context, request chat.SendRequest) chat.SendResult {
	result := chat.SendResult{LocalID: request.LocalID, ConnectionID: a.ConnectionID(), Platform: a.Platform()}
	if strings.TrimSpace(request.Text) == "" {
		result.Status, result.ProviderError = chat.DeliveryFailed, "message is empty"
		return result
	}
	body := struct {
		BroadcasterID string `json:"broadcaster_id"`
		SenderID      string `json:"sender_id"`
		Message       string `json:"message"`
		Reply         *struct {
			ParentMessageID string `json:"parent_message_id"`
		} `json:"reply_parent_message_id,omitempty"`
	}{BroadcasterID: a.config.BroadcasterID, SenderID: a.config.UserID, Message: request.Text}
	if request.ReplyToProviderID != "" {
		body.Reply = &struct {
			ParentMessageID string `json:"parent_message_id"`
		}{ParentMessageID: request.ReplyToProviderID}
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		result.Status, result.ProviderError = chat.DeliveryFailed, err.Error()
		return result
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.APIURL+"/chat/messages", strings.NewReader(string(encoded)))
	if err != nil {
		result.Status, result.ProviderError = chat.DeliveryFailed, err.Error()
		return result
	}
	a.headers(httpRequest)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(httpRequest)
	if err != nil {
		result.Status, result.Retryable, result.ProviderError = chat.DeliveryFailed, true, err.Error()
		return result
	}
	defer response.Body.Close()
	var payload struct {
		Data []struct {
			MessageID  string `json:"message_id"`
			IsSent     bool   `json:"is_sent"`
			DropReason string `json:"drop_reason"`
		} `json:"data"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		result.Status, result.Retryable, result.ProviderError = chat.DeliveryFailed, response.StatusCode >= 500, err.Error()
		return result
	}
	if response.StatusCode/100 != 2 || len(payload.Data) == 0 || !payload.Data[0].IsSent {
		result.Status = chat.DeliveryDropped
		result.DropReason = payload.Message
		if len(payload.Data) > 0 {
			result.DropReason = payload.Data[0].DropReason
		}
		result.Retryable = response.StatusCode >= 500
		return result
	}
	result.Status, result.ProviderID = chat.DeliverySent, payload.Data[0].MessageID
	return result
}

func (a *Adapter) run(ctx context.Context) {
	defer func() {
		a.mu.Lock()
		a.cancel = nil
		a.setStatusLocked(chat.StateDisconnected, "disconnected", false)
		a.mu.Unlock()
		close(a.events)
	}()
	for {
		err := a.receiveSession(ctx)
		if ctx.Err() != nil {
			return
		}
		a.emit(chat.StatusEvent{Status: a.updateStatus(chat.StateReconnecting, err.Error(), true)})
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func (a *Adapter) receiveSession(ctx context.Context) error {
	connection, _, err := websocket.Dial(ctx, a.config.EventSubURL, nil)
	if err != nil {
		return err
	}
	defer connection.Close(websocket.StatusNormalClosure, "session ended")
	var welcome eventSubEnvelope
	if err := readJSON(ctx, connection, &welcome); err != nil {
		return err
	}
	if welcome.Metadata.MessageType != "session_welcome" {
		return fmt.Errorf("Twitch EventSub expected welcome, got %q", welcome.Metadata.MessageType)
	}
	var sessionPayload struct {
		Session struct {
			ID string `json:"id"`
		} `json:"session"`
	}
	if err := json.Unmarshal(welcome.Payload, &sessionPayload); err != nil {
		return err
	}
	if err := a.subscribe(ctx, sessionPayload.Session.ID); err != nil {
		return err
	}
	a.emit(chat.StatusEvent{Status: a.updateStatus(chat.StateConnected, "connected", false)})
	for {
		var envelope eventSubEnvelope
		if err := readJSON(ctx, connection, &envelope); err != nil {
			return err
		}
		switch envelope.Metadata.MessageType {
		case "notification":
			var notification struct {
				Event json.RawMessage `json:"event"`
			}
			if err := json.Unmarshal(envelope.Payload, &notification); err != nil {
				return err
			}
			var event chatEvent
			if err := json.Unmarshal(notification.Event, &event); err != nil {
				return err
			}
			if message, ok := a.normalize(event, envelope.Metadata.MessageID); ok {
				a.emit(chat.MessageEvent{Message: message})
			}
		case "session_reconnect":
			return errors.New("Twitch EventSub requested reconnect")
		case "revocation":
			return errors.New("Twitch EventSub subscription revoked")
		}
	}
}

type eventSubEnvelope struct {
	Metadata struct {
		MessageType string `json:"message_type"`
		MessageID   string `json:"message_id"`
	} `json:"metadata"`
	Payload json.RawMessage `json:"payload"`
}

type chatEvent struct {
	MessageID       string `json:"message_id"`
	ChatterUserID   string `json:"chatter_user_id"`
	ChatterUserName string `json:"chatter_user_name"`
	Message         struct {
		Text      string `json:"text"`
		Fragments []struct {
			Type  string `json:"type"`
			Cheer struct {
				Bits int64 `json:"bits"`
			} `json:"cheer"`
		} `json:"fragments"`
	} `json:"message"`
	Color  string `json:"color"`
	Badges []struct {
		SetID string `json:"set_id"`
	} `json:"badges"`
	MessageType                 string `json:"message_type"`
	ChannelPointsCustomRewardID string `json:"channel_points_custom_reward_id"`
}

func (a *Adapter) normalize(event chatEvent, _ string) (chat.Message, bool) {
	if event.MessageID == "" || a.alreadySeen(event.MessageID) {
		return chat.Message{}, false
	}
	message := chat.Message{
		LocalID: event.MessageID, ProviderID: event.MessageID, ConnectionID: a.ConnectionID(), Platform: chat.PlatformTwitch,
		Kind: chat.MessageKindChat, Status: chat.MessageReceived, Text: event.Message.Text,
		Author:           chat.Author{ProviderID: event.ChatterUserID, Name: event.ChatterUserName, DisplayName: event.ChatterUserName},
		Priority:         chat.PriorityOrdinary,
		ProviderMetadata: map[string]any{"color": event.Color, "message_type": event.MessageType, "badges": event.Badges, "channel_points_reward_id": event.ChannelPointsCustomRewardID},
	}
	for _, fragment := range event.Message.Fragments {
		if fragment.Type == "cheermote" && fragment.Cheer.Bits > 0 {
			message.Kind, message.Priority = chat.MessageKindPaid, chat.PriorityPaid
			message.Paid = &chat.PaidEvent{AmountMinor: fragment.Cheer.Bits, Currency: "bits", DisplayAmount: fmt.Sprintf("%d bits", fragment.Cheer.Bits)}
		}
	}
	return message, true
}

func (a *Adapter) subscribe(ctx context.Context, sessionID string) error {
	body := fmt.Sprintf(`{"type":"channel.chat.message","version":"1","condition":{"broadcaster_user_id":%q,"user_id":%q},"transport":{"method":"websocket","session_id":%q}}`, a.config.BroadcasterID, a.config.UserID, sessionID)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.APIURL+"/eventsub/subscriptions", strings.NewReader(body))
	if err != nil {
		return err
	}
	a.headers(request)
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("Twitch EventSub subscription: HTTP %s", response.Status)
	}
	return nil
}

func (a *Adapter) headers(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+a.config.AccessToken)
	request.Header.Set("Client-Id", a.config.ClientID)
}

func (a *Adapter) emit(event chat.Event) {
	a.events <- event
}

func (a *Adapter) setStatusLocked(state chat.ConnectionState, detail string, retryable bool) {
	a.status = chat.ConnectionStatus{ConnectionID: a.ConnectionID(), Platform: chat.PlatformTwitch, State: state, Detail: detail, Retryable: retryable, At: time.Now()}
}

func (a *Adapter) updateStatus(state chat.ConnectionState, detail string, retryable bool) chat.ConnectionStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setStatusLocked(state, detail, retryable)
	return a.status
}

func (a *Adapter) alreadySeen(id string) bool {
	a.seenMu.Lock()
	defer a.seenMu.Unlock()
	if _, ok := a.seen[id]; ok {
		return true
	}
	a.seen[id] = struct{}{}
	a.seenIDs = append(a.seenIDs, id)
	if len(a.seenIDs) > 10_000 {
		delete(a.seen, a.seenIDs[0])
		a.seenIDs = a.seenIDs[1:]
	}
	return false
}

func readJSON(ctx context.Context, connection *websocket.Conn, value any) error {
	_, reader, err := connection.Reader(ctx)
	if err != nil {
		return err
	}
	return json.NewDecoder(reader).Decode(value)
}
