package chat

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultHistoryMessages = 10_000
	DefaultHistoryBytes    = 25 * 1024 * 1024
)

type HistoryConfig struct {
	Directory           string
	Path                string
	SessionID           string
	ConnectionID        ConnectionID
	Platform            Platform
	MaxMessages         int
	MaxBytes            int64
	PersistText         bool
	PersistAuthor       bool
	PersistProviderData bool
}

func DefaultHistoryConfig() HistoryConfig {
	return HistoryConfig{
		MaxMessages:   DefaultHistoryMessages,
		MaxBytes:      DefaultHistoryBytes,
		PersistText:   true,
		PersistAuthor: true,
	}
}

type historyEntry struct {
	Message Message
	Bytes   int
}

type historyRecord struct {
	Version      int              `json:"version"`
	LocalID      string           `json:"local_id"`
	ProviderID   string           `json:"provider_id"`
	ConnectionID ConnectionID     `json:"connection_id"`
	Platform     Platform         `json:"platform"`
	Kind         MessageKind      `json:"kind"`
	Status       MessageStatus    `json:"status"`
	Author       *Author          `json:"author,omitempty"`
	Text         string           `json:"text,omitempty"`
	SentAt       string           `json:"sent_at"`
	UpdatedAt    string           `json:"updated_at,omitempty"`
	Paid         *PaidEvent       `json:"paid,omitempty"`
	Membership   *MembershipEvent `json:"membership,omitempty"`
	Priority     PriorityClass    `json:"priority,omitempty"`
	Highlight    bool             `json:"highlight,omitempty"`
	VisibleUntil string           `json:"visible_until,omitempty"`
	ProviderData map[string]any   `json:"provider_data,omitempty"`
}

type History struct {
	mu        sync.RWMutex
	config    HistoryConfig
	entries   []historyEntry
	bytes     int64
	file      *os.File
	fileBytes int64
}

func NewHistory(config HistoryConfig) (*History, error) {
	if config.MaxMessages <= 0 {
		return nil, errors.New("history max messages must be positive")
	}
	if config.MaxBytes <= 0 {
		return nil, errors.New("history max bytes must be positive")
	}
	if config.SessionID == "" || config.ConnectionID == "" || config.Platform == "" {
		return nil, errors.New("history session, connection, and platform are required")
	}
	if config.Directory == "" {
		return nil, errors.New("history directory is required")
	}

	if err := os.MkdirAll(config.Directory, 0o700); err != nil {
		return nil, fmt.Errorf("create history directory: %w", err)
	}
	path := filepath.Join(config.Directory, historyFilename(config))
	if config.Path != "" {
		path = config.Path
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open history log: %w", err)
	}
	history := &History{config: config, file: file}
	if err := history.load(file); err != nil {
		file.Close()
		return nil, err
	}
	return history, nil
}

func (h *History) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.file == nil {
		return nil
	}
	err := h.file.Close()
	h.file = nil
	return err
}

func (h *History) Append(message Message) error {
	record, err := h.recordFor(message)
	if err != nil {
		return err
	}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode history message: %w", err)
	}
	line = append(line, '\n')
	if int64(len(line)) > h.config.MaxBytes {
		return errors.New("history message exceeds maximum log size")
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.file == nil {
		return errors.New("history is closed")
	}
	if h.fileBytes+int64(len(line)) > h.config.MaxBytes {
		if err := h.rotateLocked(); err != nil {
			return err
		}
	}
	if _, err := h.file.Write(line); err != nil {
		return fmt.Errorf("write history message: %w", err)
	}
	h.fileBytes += int64(len(line))
	h.entries = append(h.entries, historyEntry{Message: message, Bytes: len(line)})
	h.bytes += int64(len(line))
	h.trimLocked()
	return nil
}

func (h *History) Page(offset, limit int) []Message {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || offset >= len(h.entries) {
		return nil
	}
	end := offset + limit
	if end > len(h.entries) {
		end = len(h.entries)
	}
	page := make([]Message, end-offset)
	for i := range page {
		page[i] = h.entries[offset+i].Message
	}
	return page
}

func (h *History) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.entries)
}

func (h *History) load(file *os.File) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("seek history log: %w", err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat history log: %w", err)
	}
	h.fileBytes = fileInfo.Size()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), int(h.config.MaxBytes))
	for scanner.Scan() {
		line := scanner.Bytes()
		var record historyRecord
		if err := json.Unmarshal(line, &record); err != nil || record.Version != 1 {
			continue
		}
		message := record.message()
		h.entries = append(h.entries, historyEntry{Message: message, Bytes: len(line) + 1})
		h.bytes += int64(len(line) + 1)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read history log: %w", err)
	}
	h.trimLocked()
	if _, err := file.Seek(0, 2); err != nil {
		return fmt.Errorf("seek history log end: %w", err)
	}
	return nil
}

func (h *History) recordFor(message Message) (historyRecord, error) {
	record := historyRecord{
		Version:      1,
		LocalID:      message.LocalID,
		ProviderID:   message.ProviderID,
		ConnectionID: message.ConnectionID,
		Platform:     message.Platform,
		Kind:         message.Kind,
		Status:       message.Status,
		SentAt:       message.SentAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		Priority:     message.Priority,
		Highlight:    message.Highlight,
		Paid:         message.Paid,
		Membership:   message.Membership,
	}
	if !message.UpdatedAt.IsZero() {
		record.UpdatedAt = message.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	if !message.VisibleUntil.IsZero() {
		record.VisibleUntil = message.VisibleUntil.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}
	if h.config.PersistText {
		record.Text = message.Text
	}
	if h.config.PersistAuthor {
		author := message.Author
		record.Author = &author
	}
	if h.config.PersistProviderData {
		record.ProviderData = message.ProviderMetadata
	}
	return record, nil
}

func (h *History) trimLocked() {
	for len(h.entries) > h.config.MaxMessages || h.bytes > h.config.MaxBytes {
		if len(h.entries) == 0 {
			return
		}
		h.bytes -= int64(h.entries[0].Bytes)
		h.entries = h.entries[1:]
	}
}

func (h *History) rotateLocked() error {
	if err := h.file.Close(); err != nil {
		return fmt.Errorf("close history log for rotation: %w", err)
	}
	path := filepath.Join(h.config.Directory, historyFilename(h.config))
	if h.config.Path != "" {
		path = h.config.Path
	}
	rotated := path + ".1"
	if err := os.Remove(rotated); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove rotated history log: %w", err)
	}
	if err := os.Rename(path, rotated); err != nil {
		return fmt.Errorf("rotate history log: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("reopen history log: %w", err)
	}
	h.file = file
	h.fileBytes = 0
	return nil
}

func historyFilename(config HistoryConfig) string {
	if config.Path != "" {
		return config.Path
	}
	return strings.Join([]string{
		safeHistoryPart(config.SessionID),
		safeHistoryPart(string(config.Platform)),
		safeHistoryPart(string(config.ConnectionID)),
	}, "-") + ".jsonl"
}

func safeHistoryPart(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}

func (r historyRecord) message() Message {
	message := Message{
		LocalID:          r.LocalID,
		ProviderID:       r.ProviderID,
		ConnectionID:     r.ConnectionID,
		Platform:         r.Platform,
		Kind:             r.Kind,
		Status:           r.Status,
		Text:             r.Text,
		Paid:             r.Paid,
		Membership:       r.Membership,
		Priority:         r.Priority,
		Highlight:        r.Highlight,
		ProviderMetadata: r.ProviderData,
	}
	if r.Author != nil {
		message.Author = *r.Author
	}
	message.SentAt, _ = time.Parse(time.RFC3339Nano, r.SentAt)
	if r.UpdatedAt != "" {
		message.UpdatedAt, _ = time.Parse(time.RFC3339Nano, r.UpdatedAt)
	}
	if r.VisibleUntil != "" {
		message.VisibleUntil, _ = time.Parse(time.RFC3339Nano, r.VisibleUntil)
	}
	return message
}
