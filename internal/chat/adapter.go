package chat

import (
	"context"
	"time"
	"unicode/utf8"
)

// Platform identifies the service that owns a connection or message.
type Platform string

const (
	PlatformTwitch  Platform = "twitch"
	PlatformYouTube Platform = "youtube"
)

// ConnectionID identifies one configured destination. It is distinct from a
// provider ID because provider IDs are not guaranteed to be globally unique.
type ConnectionID string

type ConnectionState string

const (
	StateDisconnected ConnectionState = "disconnected"
	StateConnecting   ConnectionState = "connecting"
	StateConnected    ConnectionState = "connected"
	StateReconnecting ConnectionState = "reconnecting"
	StateStopping     ConnectionState = "stopping"
	StateFailed       ConnectionState = "failed"
)

// Capabilities describe optional provider behavior. An adapter must report
// unsupported capabilities as false rather than making the UI infer them.
type Capabilities struct {
	ReceiveChat        bool
	SendChat           bool
	MessageBadges      bool
	MessageEmotes      bool
	PaidMessages       bool
	MembershipMessages bool
	FollowerState      bool
	FirstTimeState     bool
	MessageUpdates     bool
	CursorResume       bool
	// MaxMessageLength is the provider's chat message limit in Unicode code points.
	// Zero means that the provider does not advertise a known limit.
	MaxMessageLength int
}

func MessageLength(text string) int { return utf8.RuneCountInString(text) }

type ConnectionStatus struct {
	ConnectionID ConnectionID
	Platform     Platform
	State        ConnectionState
	Detail       string
	Retryable    bool
	At           time.Time
}

type Author struct {
	ProviderID   string
	Name         string
	DisplayName  string
	IsModerator  bool
	IsSubscriber bool
	IsMember     bool
	IsFollower   bool
	IsFirstTime  bool
}

type MessageKind string

const (
	MessageKindChat       MessageKind = "chat"
	MessageKindPaid       MessageKind = "paid"
	MessageKindMembership MessageKind = "membership"
)

type MessageStatus string

const (
	MessageReceived MessageStatus = "received"
	MessageUpdated  MessageStatus = "updated"
	MessageDeleted  MessageStatus = "deleted"
)

type PriorityClass string

const (
	PriorityOrdinary   PriorityClass = "ordinary"
	PriorityFirstTime  PriorityClass = "first_time"
	PriorityFollower   PriorityClass = "follower"
	PrioritySubscriber PriorityClass = "subscriber"
	PriorityModerator  PriorityClass = "moderator"
	PriorityPaid       PriorityClass = "paid"
)

type PaidEvent struct {
	AmountMinor   int64
	Currency      string
	DisplayAmount string
	IsGift        bool
}

type MembershipEvent struct {
	Level     string
	Months    int
	IsGift    bool
	GiftCount int
}

// Message is the provider-independent chat representation. ProviderMetadata
// is intentionally opaque so provider-specific fields do not leak into the
// core model.
type Message struct {
	LocalID          string
	ProviderID       string
	ConnectionID     ConnectionID
	Platform         Platform
	Kind             MessageKind
	Status           MessageStatus
	Author           Author
	Text             string
	SentAt           time.Time
	UpdatedAt        time.Time
	Paid             *PaidEvent
	Membership       *MembershipEvent
	Priority         PriorityClass
	Highlight        bool
	VisibleUntil     time.Time
	ProviderMetadata map[string]any
}

type Event interface{ isEvent() }

type StatusEvent struct{ Status ConnectionStatus }

func (StatusEvent) isEvent() {}

type MessageEvent struct{ Message Message }

func (MessageEvent) isEvent() {}

type MessageUpdateEvent struct {
	ProviderID string
	Message    Message
}

func (MessageUpdateEvent) isEvent() {}

type SendRequest struct {
	LocalID           string
	Text              string
	ReplyToProviderID string
}

type DeliveryStatus string

const (
	DeliverySent    DeliveryStatus = "sent"
	DeliveryDropped DeliveryStatus = "dropped"
	DeliveryFailed  DeliveryStatus = "failed"
)

type SendResult struct {
	LocalID       string
	ProviderID    string
	ConnectionID  ConnectionID
	Platform      Platform
	Status        DeliveryStatus
	Retryable     bool
	DropReason    string
	ProviderError string
}

// Adapter owns provider authentication, reconnects, backoff, cursor resume,
// and duplicate suppression. Its event stream is read by the application and
// never by the renderer directly.
type Adapter interface {
	ConnectionID() ConnectionID
	Platform() Platform
	Capabilities() Capabilities
	Status() ConnectionStatus
	Events() <-chan Event
	Connect(context.Context) error
	Disconnect(context.Context) error
	Send(context.Context, SendRequest) SendResult
}
