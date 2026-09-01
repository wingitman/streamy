package chat

import (
	"errors"
	"time"
)

const defaultVisibleFor = 8 * time.Second

type PriorityRule struct {
	Weight     int
	VisibleFor time.Duration
	Highlight  bool
}

type PriorityPolicy map[PriorityClass]PriorityRule

func DefaultPriorityPolicy() PriorityPolicy {
	return PriorityPolicy{
		PriorityOrdinary:   {Weight: 1, VisibleFor: defaultVisibleFor},
		PriorityFirstTime:  {Weight: 2, VisibleFor: 12 * time.Second, Highlight: true},
		PriorityFollower:   {Weight: 2, VisibleFor: 12 * time.Second},
		PrioritySubscriber: {Weight: 3, VisibleFor: 15 * time.Second, Highlight: true},
		PriorityModerator:  {Weight: 4, VisibleFor: 20 * time.Second, Highlight: true},
		PriorityPaid:       {Weight: 5, VisibleFor: 25 * time.Second, Highlight: true},
	}
}

type BurstConfig struct {
	MaxMessages int
	Window      time.Duration
}

func DefaultBurstConfig() BurstConfig {
	return BurstConfig{MaxMessages: 30, Window: 5 * time.Second}
}

type RenderDecision struct {
	Message        Message
	RenderNow      bool
	Suppressed     bool
	PriorityWeight int
	VisibleUntil   time.Time
	Highlight      bool
}

type BurstController struct {
	config  BurstConfig
	policy  PriorityPolicy
	now     func() time.Time
	renders []time.Time
}

func NewBurstController(config BurstConfig, policy PriorityPolicy, now func() time.Time) (*BurstController, error) {
	if config.MaxMessages <= 0 {
		return nil, errors.New("burst max messages must be positive")
	}
	if config.Window <= 0 {
		return nil, errors.New("burst window must be positive")
	}
	if now == nil {
		now = time.Now
	}
	if policy == nil {
		policy = DefaultPriorityPolicy()
	}
	return &BurstController{config: config, policy: policy, now: now}, nil
}

// Offer applies the live render budget. Every offered message is returned to
// the caller, including suppressed messages, so history can retain it.
func (c *BurstController) Offer(message Message) RenderDecision {
	now := c.now()
	c.prune(now)
	rule := c.policy[message.Priority]
	if rule.VisibleFor <= 0 {
		rule.VisibleFor = defaultVisibleFor
	}

	decision := RenderDecision{
		Message:        message,
		PriorityWeight: rule.Weight,
		VisibleUntil:   now.Add(rule.VisibleFor),
		Highlight:      message.Highlight || rule.Highlight,
	}
	if len(c.renders) >= c.config.MaxMessages {
		decision.Suppressed = true
		return decision
	}

	c.renders = append(c.renders, now)
	decision.RenderNow = true
	return decision
}

func (c *BurstController) prune(now time.Time) {
	cutoff := now.Add(-c.config.Window)
	first := 0
	for first < len(c.renders) && !c.renders[first].After(cutoff) {
		first++
	}
	if first > 0 {
		c.renders = append(c.renders[:0], c.renders[first:]...)
	}
}
