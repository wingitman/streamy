package youtube

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/wingitman/streamy/internal/chat"
)

const defaultAPIURL = "https://youtube.googleapis.com/youtube/v3"

type Config struct {
	ConnectionID chat.ConnectionID
	LiveChatID   string
	ClientID     string
	AccessToken  string
	APIURL       string
	HTTPClient   *http.Client
}

type Adapter struct {
	config  Config
	client  *http.Client
	mu      sync.RWMutex
	status  chat.ConnectionStatus
	cancel  context.CancelFunc
	done    chan struct{}
	events  chan chat.Event
	seenMu  sync.Mutex
	seen    map[string]struct{}
	seenIDs []string
}

func New(config Config) (*Adapter, error) {
	if config.ConnectionID == "" || config.LiveChatID == "" || config.ClientID == "" || config.AccessToken == "" {
		return nil, errors.New("YouTube connection, live chat, client, and token settings are required")
	}
	if config.APIURL == "" {
		config.APIURL = defaultAPIURL
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return &Adapter{
		config: config, client: config.HTTPClient,
		status: chat.ConnectionStatus{ConnectionID: config.ConnectionID, Platform: chat.PlatformYouTube, State: chat.StateDisconnected},
		events: make(chan chat.Event, 256), seen: make(map[string]struct{}),
	}, nil
}

func (a *Adapter) ConnectionID() chat.ConnectionID { return a.config.ConnectionID }
func (a *Adapter) Platform() chat.Platform         { return chat.PlatformYouTube }
func (a *Adapter) Capabilities() chat.Capabilities {
	return chat.Capabilities{ReceiveChat: true, SendChat: true, PaidMessages: true, MembershipMessages: true, MessageUpdates: true, CursorResume: true}
}
func (a *Adapter) Status() chat.ConnectionStatus { a.mu.RLock(); defer a.mu.RUnlock(); return a.status }
func (a *Adapter) Events() <-chan chat.Event     { return a.events }

func (a *Adapter) Connect(parent context.Context) error {
	a.mu.Lock()
	if a.cancel != nil {
		a.mu.Unlock()
		return errors.New("YouTube adapter is already connected")
	}
	ctx, cancel := context.WithCancel(parent)
	a.cancel, a.done = cancel, make(chan struct{})
	a.setStatusLocked(chat.StateConnecting, "connecting", true)
	done := a.done
	a.mu.Unlock()
	go func() { defer close(done); a.run(ctx) }()
	return nil
}

func (a *Adapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	cancel, done := a.cancel, a.done
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
		Snippet struct {
			LiveChatID string `json:"liveChatId"`
			Type       string `json:"type"`
			Text       struct {
				MessageText string `json:"messageText"`
			} `json:"textMessageDetails"`
		} `json:"snippet"`
	}{}
	body.Snippet.LiveChatID, body.Snippet.Type, body.Snippet.Text.MessageText = a.config.LiveChatID, "textMessageEvent", request.Text
	encoded, err := json.Marshal(body)
	if err != nil {
		result.Status, result.ProviderError = chat.DeliveryFailed, err.Error()
		return result
	}
	endpoint := a.config.APIURL + "/liveChat/messages?part=snippet"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(encoded)))
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
	var item youtubeItem
	if err := json.NewDecoder(response.Body).Decode(&item); err != nil {
		result.Status, result.Retryable, result.ProviderError = chat.DeliveryFailed, response.StatusCode >= 500, err.Error()
		return result
	}
	if response.StatusCode/100 != 2 || item.ID == "" {
		result.Status, result.Retryable, result.ProviderError = chat.DeliveryFailed, response.StatusCode >= 500, fmt.Sprintf("YouTube API returned %s", response.Status)
		return result
	}
	result.Status, result.ProviderID = chat.DeliverySent, item.ID
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
		err := a.receiveStream(ctx)
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

func (a *Adapter) receiveStream(ctx context.Context) error {
	query := url.Values{"liveChatId": {a.config.LiveChatID}, "part": {"snippet,authorDetails"}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.config.APIURL+"/liveChat/messages/streamList?"+query.Encode(), nil)
	if err != nil {
		return err
	}
	a.headers(request)
	response, err := a.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("YouTube live chat stream: HTTP %s", response.Status)
	}
	a.emit(chat.StatusEvent{Status: a.updateStatus(chat.StateConnected, "connected", false)})
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item youtubeItem
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return err
		}
		if message, ok := a.normalize(item); ok {
			a.emit(chat.MessageEvent{Message: message})
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

type youtubeItem struct {
	ID      string `json:"id"`
	Snippet struct {
		Type           string    `json:"type"`
		PublishedAt    time.Time `json:"publishedAt"`
		DisplayMessage string    `json:"displayMessage"`
		SuperChat      struct {
			AmountMicros        int64  `json:"amountMicros"`
			Currency            string `json:"currency"`
			AmountDisplayString string `json:"amountDisplayString"`
		} `json:"superChatDetails"`
		Membership struct {
			MemberMonths      int  `json:"memberMonth"`
			HasGiftMembership bool `json:"hasGiftMembership"`
		} `json:"memberMilestoneChatDetails"`
	} `json:"snippet"`
	Author struct {
		ID        string `json:"channelId"`
		Name      string `json:"displayName"`
		Moderator bool   `json:"isChatModerator"`
		Sponsor   bool   `json:"isChatSponsor"`
		Owner     bool   `json:"isChatOwner"`
	} `json:"authorDetails"`
}

func (a *Adapter) normalize(item youtubeItem) (chat.Message, bool) {
	if item.ID == "" || a.alreadySeen(item.ID) {
		return chat.Message{}, false
	}
	message := chat.Message{LocalID: item.ID, ProviderID: item.ID, ConnectionID: a.ConnectionID(), Platform: chat.PlatformYouTube, Kind: chat.MessageKindChat, Status: chat.MessageReceived, Text: item.Snippet.DisplayMessage, SentAt: item.Snippet.PublishedAt, Author: chat.Author{ProviderID: item.Author.ID, DisplayName: item.Author.Name, Name: item.Author.Name, IsModerator: item.Author.Moderator, IsSubscriber: item.Author.Sponsor}, Priority: chat.PriorityOrdinary}
	if item.Snippet.SuperChat.AmountMicros > 0 {
		message.Kind, message.Priority = chat.MessageKindPaid, chat.PriorityPaid
		message.Paid = &chat.PaidEvent{AmountMinor: item.Snippet.SuperChat.AmountMicros, Currency: item.Snippet.SuperChat.Currency, DisplayAmount: item.Snippet.SuperChat.AmountDisplayString}
	}
	if item.Snippet.Type == "memberMilestoneChatEvent" || item.Snippet.Type == "newSponsorEvent" || item.Snippet.Type == "membershipGiftingEvent" {
		message.Kind, message.Priority = chat.MessageKindMembership, chat.PrioritySubscriber
		message.Membership = &chat.MembershipEvent{Months: item.Snippet.Membership.MemberMonths, IsGift: item.Snippet.Membership.HasGiftMembership}
	}
	return message, true
}

func (a *Adapter) headers(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+a.config.AccessToken)
}
func (a *Adapter) emit(event chat.Event) { a.events <- event }
func (a *Adapter) setStatusLocked(state chat.ConnectionState, detail string, retryable bool) {
	a.status = chat.ConnectionStatus{ConnectionID: a.ConnectionID(), Platform: chat.PlatformYouTube, State: state, Detail: detail, Retryable: retryable, At: time.Now()}
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
