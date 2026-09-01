package channels

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wingitman/streamy/internal/chat"
)

type View string

const (
	ViewCombined View = "combined"
	ViewTwitch   View = "twitch"
	ViewYouTube  View = "youtube"
)

const AllTargets chat.ConnectionID = "*"

type DeliveryState string

const (
	DeliveryPending  DeliveryState = "pending"
	DeliverySent     DeliveryState = "sent"
	DeliveryFailed   DeliveryState = "failed"
	DeliveryRetrying DeliveryState = "retrying"
)

type Target struct {
	ConnectionID chat.ConnectionID
	Platform     chat.Platform
	Label        string
	Status       chat.ConnectionStatus
}

type Delivery struct {
	LocalID      string
	ConnectionID chat.ConnectionID
	Platform     chat.Platform
	Text         string
	State        DeliveryState
	Attempts     int
	Result       chat.SendResult
}

type SendCommand struct {
	Adapter chat.Adapter
	Request chat.SendRequest
}

type Model struct {
	view       View
	filter     string
	composer   string
	target     chat.ConnectionID
	targets    []Target
	adapters   map[chat.ConnectionID]chat.Adapter
	messages   []chat.RenderDecision
	deliveries map[string]Delivery
	nextID     int64
}

func NewModel(adapters []chat.Adapter) *Model {
	model := &Model{
		view:       ViewCombined,
		target:     AllTargets,
		adapters:   make(map[chat.ConnectionID]chat.Adapter, len(adapters)),
		deliveries: make(map[string]Delivery),
	}
	for _, adapter := range adapters {
		id := adapter.ConnectionID()
		model.adapters[id] = adapter
		model.targets = append(model.targets, Target{
			ConnectionID: id,
			Platform:     adapter.Platform(),
			Label:        string(id),
			Status:       adapter.Status(),
		})
	}
	return model
}

func (m *Model) AddAdapter(adapter chat.Adapter) {
	if m == nil || adapter == nil {
		return
	}
	id := adapter.ConnectionID()
	if _, exists := m.adapters[id]; exists {
		return
	}
	m.adapters[id] = adapter
	m.targets = append(m.targets, Target{ConnectionID: id, Platform: adapter.Platform(), Label: string(id), Status: adapter.Status()})
}

func (m *Model) RemoveAdapter(id chat.ConnectionID) {
	if m == nil {
		return
	}
	delete(m.adapters, id)
	for i, target := range m.targets {
		if target.ConnectionID == id {
			m.targets = append(m.targets[:i], m.targets[i+1:]...)
			break
		}
	}
	if m.target == id {
		m.target = AllTargets
	}
}

func (m *Model) View() View { return m.view }

func (m *Model) SetView(view View) error {
	switch view {
	case ViewCombined, ViewTwitch, ViewYouTube:
		m.view = view
		return nil
	default:
		return fmt.Errorf("unknown channels view %q", view)
	}
}

func (m *Model) SetFilter(filter string) { m.filter = strings.TrimSpace(filter) }

func (m *Model) SetComposer(text string) { m.composer = text }

func (m *Model) Composer() string { return m.composer }

// MessageLimit returns the strictest limit among the currently selected targets.
func (m *Model) MessageLimit() (int, bool) {
	limit := 0
	for _, adapter := range m.selectedAdapters() {
		candidate := adapter.Capabilities().MaxMessageLength
		if candidate > 0 && (limit == 0 || candidate < limit) {
			limit = candidate
		}
	}
	return limit, limit > 0
}

func (m *Model) Targets() []Target {
	targets := make([]Target, len(m.targets))
	copy(targets, m.targets)
	return targets
}

func (m *Model) SelectedTarget() chat.ConnectionID { return m.target }

func (m *Model) SelectTarget(target chat.ConnectionID) error {
	if target == AllTargets {
		m.target = target
		return nil
	}
	if _, ok := m.adapters[target]; !ok {
		return fmt.Errorf("unknown target %q", target)
	}
	m.target = target
	return nil
}

func (m *Model) AddMessage(decision chat.RenderDecision) bool {
	const maxViewMessages = 10_000
	if decision.Message.ProviderID != "" {
		for _, existing := range m.messages {
			if existing.Message.ConnectionID == decision.Message.ConnectionID && existing.Message.ProviderID == decision.Message.ProviderID {
				return false
			}
		}
	}
	m.messages = append(m.messages, decision)
	if len(m.messages) > maxViewMessages {
		m.messages = m.messages[len(m.messages)-maxViewMessages:]
	}
	return true
}

func (m *Model) UpdateMessage(update chat.MessageUpdateEvent) {
	if update.Message.ProviderID == "" {
		update.Message.ProviderID = update.ProviderID
	}
	for index := range m.messages {
		message := m.messages[index].Message
		if message.ConnectionID == update.Message.ConnectionID && message.ProviderID == update.ProviderID {
			if update.Message.Status == chat.MessageDeleted {
				update.Message.Text = "[message deleted]"
			}
			m.messages[index] = chat.RenderDecision{RenderNow: true, Message: update.Message}
			return
		}
	}
	if update.Message.Status != chat.MessageDeleted {
		m.AddMessage(chat.RenderDecision{RenderNow: true, Message: update.Message})
	}
}

func (m *Model) Messages() []chat.Message {
	messages := make([]chat.Message, 0, len(m.messages))
	for _, decision := range m.messages {
		if decision.Suppressed || !m.matchesView(decision.Message) || !matchesFilter(decision.Message, m.filter) {
			continue
		}
		messages = append(messages, decision.Message)
	}
	return messages
}

func (m *Model) SuppressedPage(offset, limit int) []chat.Message {
	messages := make([]chat.Message, 0)
	for _, decision := range m.messages {
		if decision.Suppressed && m.matchesView(decision.Message) && matchesFilter(decision.Message, m.filter) {
			messages = append(messages, decision.Message)
		}
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || offset >= len(messages) {
		return nil
	}
	end := offset + limit
	if end > len(messages) {
		end = len(messages)
	}
	return messages[offset:end]
}

func (m *Model) Submit() ([]SendCommand, error) {
	text := strings.TrimSpace(m.composer)
	if text == "" {
		return nil, errors.New("cannot send an empty message")
	}
	if len(m.adapters) == 0 {
		return nil, errors.New("no chat connections are configured")
	}
	targets := m.selectedAdapters()
	if len(targets) == 0 {
		return nil, errors.New("selected target is unavailable")
	}
	length := chat.MessageLength(text)
	for _, adapter := range targets {
		limit := adapter.Capabilities().MaxMessageLength
		if limit > 0 && length > limit {
			return nil, fmt.Errorf("%s: message is %d/%d characters", adapter.ConnectionID(), length, limit)
		}
	}

	m.nextID++
	localID := fmt.Sprintf("local-%d", m.nextID)
	commands := make([]SendCommand, 0, len(targets))
	for _, adapter := range targets {
		request := chat.SendRequest{LocalID: localID, Text: text}
		commands = append(commands, SendCommand{Adapter: adapter, Request: request})
		id := adapter.ConnectionID()
		m.deliveries[deliveryKey(localID, id)] = Delivery{
			LocalID: localID, ConnectionID: id, Platform: adapter.Platform(), Text: text,
			State: DeliveryPending, Attempts: 1,
		}
	}
	m.composer = ""
	return commands, nil
}

func (m *Model) ApplyResult(result chat.SendResult) error {
	key := deliveryKey(result.LocalID, result.ConnectionID)
	delivery, ok := m.deliveries[key]
	if !ok {
		return fmt.Errorf("unknown delivery %q", key)
	}
	delivery.Result = result
	if result.Status == chat.DeliverySent {
		delivery.State = DeliverySent
	} else {
		delivery.State = DeliveryFailed
	}
	m.deliveries[key] = delivery
	return nil
}

func (m *Model) Retry(localID string, connectionID chat.ConnectionID) (SendCommand, error) {
	key := deliveryKey(localID, connectionID)
	delivery, ok := m.deliveries[key]
	if !ok {
		return SendCommand{}, fmt.Errorf("unknown delivery %q", key)
	}
	if delivery.State != DeliveryFailed || !delivery.Result.Retryable {
		return SendCommand{}, errors.New("delivery is not retryable")
	}
	adapter, ok := m.adapters[connectionID]
	if !ok {
		return SendCommand{}, errors.New("target is unavailable")
	}
	delivery.State = DeliveryRetrying
	delivery.Attempts++
	m.deliveries[key] = delivery
	return SendCommand{Adapter: adapter, Request: chat.SendRequest{LocalID: localID, Text: delivery.Text}}, nil
}

func (m *Model) Deliveries() []Delivery {
	deliveries := make([]Delivery, 0, len(m.deliveries))
	for _, delivery := range m.deliveries {
		deliveries = append(deliveries, delivery)
	}
	sort.Slice(deliveries, func(i, j int) bool {
		return deliveryNumber(deliveries[i].LocalID) < deliveryNumber(deliveries[j].LocalID)
	})
	return deliveries
}

func (m *Model) LatestRetryable() (SendCommand, error) {
	var latest Delivery
	found := false
	for _, delivery := range m.deliveries {
		if delivery.State != DeliveryFailed || !delivery.Result.Retryable {
			continue
		}
		if !found || deliveryNumber(delivery.LocalID) > deliveryNumber(latest.LocalID) {
			latest, found = delivery, true
		}
	}
	if !found {
		return SendCommand{}, errors.New("no retryable delivery")
	}
	return m.Retry(latest.LocalID, latest.ConnectionID)
}

func deliveryNumber(localID string) int64 {
	number, _ := strconv.ParseInt(strings.TrimPrefix(localID, "local-"), 10, 64)
	return number
}

func (m *Model) Execute(ctx context.Context, command SendCommand) chat.SendResult {
	return command.Adapter.Send(ctx, command.Request)
}

func (m *Model) selectedAdapters() []chat.Adapter {
	if m.target != AllTargets {
		adapter := m.adapters[m.target]
		if adapter == nil {
			return nil
		}
		return []chat.Adapter{adapter}
	}
	targets := make([]chat.Adapter, 0, len(m.targets))
	for _, target := range m.targets {
		targets = append(targets, m.adapters[target.ConnectionID])
	}
	return targets
}

func (m *Model) matchesView(message chat.Message) bool {
	switch m.view {
	case ViewTwitch:
		return message.Platform == chat.PlatformTwitch
	case ViewYouTube:
		return message.Platform == chat.PlatformYouTube
	default:
		return true
	}
}

func matchesFilter(message chat.Message, filter string) bool {
	if filter == "" {
		return true
	}
	needle := strings.ToLower(filter)
	return strings.Contains(strings.ToLower(message.Text), needle) ||
		strings.Contains(strings.ToLower(message.Author.DisplayName), needle)
}

func deliveryKey(localID string, connectionID chat.ConnectionID) string {
	return localID + "\x00" + string(connectionID)
}
