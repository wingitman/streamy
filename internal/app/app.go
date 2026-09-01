// Package app contains Streamy's Delbysoft TUI application model.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/wingitman/streamy/internal/auth"
	"github.com/wingitman/streamy/internal/channels"
	"github.com/wingitman/streamy/internal/chat"
	"github.com/wingitman/streamy/internal/config"
	"github.com/wingitman/streamy/internal/history"
	"github.com/wingitman/streamy/internal/platform"
	"github.com/wingitman/streamy/internal/ui"
	"github.com/wingitman/streamy/internal/update"
	"github.com/wingitman/streamy/internal/version"
)

type mode int

const (
	modeNormal mode = iota
	modeHelp
	modeTheme
	modeHistory
	modeUpdates
	modeIntegrations
	modeIntegrationSetup
)

type Model struct {
	name              string
	cfg               config.Config
	styles            ui.Styles
	width, height     int
	items             []string
	cursor, scroll    int
	status            string
	current           mode
	themeNames        []string
	themeCursor       int
	history           []history.Entry
	update            update.Result
	layout            Layout
	channels          *channels.Model
	burst             *chat.BurstController
	adapters          []chat.Adapter
	histories         map[chat.ConnectionID]*chat.History
	composer          string
	filterMode        bool
	filter            string
	statuses          map[chat.ConnectionID]chat.ConnectionStatus
	reconnecting      map[chat.ConnectionID]bool
	integrationCursor int
	setupPlatform     chat.Platform
	setupField        int
	setupValues       [4]string
	setupClientIDKept bool
	setupSecretKept   bool
	setupEnabled      bool
	setupEditing      bool
	setupBeforeEdit   string
	connector         IntegrationConnector
	warnings          []string
}

type configMsg struct {
	cfg config.Config
	err error
}
type clipboardMsg struct{ err error }
type historyMsg struct {
	entries []history.Entry
	err     error
}
type updateMsg struct{ result update.Result }
type clearStatusMsg struct{}
type eventMsg struct {
	adapter chat.Adapter
	event   chat.Event
}
type sendMsg struct{ result chat.SendResult }
type reconnectMsg struct {
	id  chat.ConnectionID
	err error
}
type errorMsg struct{ err error }
type editorMsg struct{ err error }
type integrationMsg struct {
	cfg           config.Config
	err           error
	authenticated bool
	authErr       error
	adapter       chat.Adapter
	history       *chat.History
	connectionID  chat.ConnectionID
	enabled       bool
}

type IntegrationConnector func(context.Context, config.Config, auth.ConnectionConfig, auth.Credential) (chat.Adapter, *chat.History, error)

func New(name string, cfg config.Config, adapters []chat.Adapter, histories map[chat.ConnectionID]*chat.History) (Model, error) {
	names, _ := config.ThemeNames(cfg)
	burst, err := chat.NewBurstController(chat.DefaultBurstConfig(), chat.DefaultPriorityPolicy(), nil)
	if err != nil {
		return Model{}, err
	}
	statuses := make(map[chat.ConnectionID]chat.ConnectionStatus, len(adapters))
	for _, adapter := range adapters {
		statuses[adapter.ConnectionID()] = adapter.Status()
	}
	model := Model{name: name, cfg: cfg, styles: ui.NewStyles(config.ResolveTheme(cfg)), themeNames: names,
		channels: channels.NewModel(adapters), burst: burst, adapters: adapters, histories: histories,
		statuses: statuses, reconnecting: make(map[chat.ConnectionID]bool)}
	model.reflow()
	return model, nil
}

func (m *Model) SetIntegrationConnector(connector IntegrationConnector) { m.connector = connector }

func (m *Model) SetWarnings(warnings []string) {
	m.warnings = append([]string(nil), warnings...)
}
func (m Model) Init() tea.Cmd {
	commands := []tea.Cmd{m.checkUpdate()}
	for _, adapter := range m.adapters {
		a := adapter
		commands = append(commands, func() tea.Msg {
			if err := a.Connect(context.Background()); err != nil {
				return errorMsg{err}
			}
			return nil
		}, waitEvent(a))
	}
	return tea.Batch(commands...)
}

func (m Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.reflow()
	case configMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
		} else {
			m.cfg = msg.cfg
			m.styles = ui.NewStyles(config.ResolveTheme(m.cfg))
			m.themeNames, _ = config.ThemeNames(m.cfg)
			m.status = "configuration reloaded"
		}
		m.reflow()
	case clipboardMsg:
		if msg.err != nil {
			m.status = "clipboard: " + msg.err.Error()
		} else {
			m.status = "copied to clipboard"
		}
	case historyMsg:
		if msg.err != nil {
			m.status = "history: " + msg.err.Error()
		} else {
			m.history = msg.entries
			m.status = fmt.Sprintf("%d history entries", len(m.history))
		}
	case updateMsg:
		m.update = msg.result
		if msg.result.Error != nil {
			m.status = "update check: " + msg.result.Error.Error()
		} else if msg.result.Available {
			m.status = "update available: " + msg.result.Latest
		} else {
			m.status = "up to date"
		}
	case clearStatusMsg:
		m.status = ""
	case errorMsg:
		m.status = msg.err.Error()
	case editorMsg:
		if msg.err != nil {
			m.status = "editor: " + msg.err.Error()
		} else {
			return m, m.reloadConfig()
		}
	case integrationMsg:
		if msg.err != nil {
			m.status = "integration setup: " + msg.err.Error()
		} else {
			m.cfg = msg.cfg
			m.current = modeNormal
			if msg.authErr != nil {
				m.status = "saved integration; authentication: " + msg.authErr.Error()
			} else if msg.authenticated {
				m.status = "saved integration and authenticated; restart Streamy to connect " + m.setupValues[0]
			} else {
				m.status = "saved integration"
			}
			if !msg.enabled && msg.connectionID != "" {
				for _, adapter := range m.adapters {
					if adapter.ConnectionID() == msg.connectionID {
						_ = adapter.Disconnect(context.Background())
						m.channels.RemoveAdapter(msg.connectionID)
						delete(m.statuses, msg.connectionID)
						break
					}
				}
			} else if msg.adapter != nil {
				replaced := false
				for i, adapter := range m.adapters {
					if adapter.ConnectionID() == msg.adapter.ConnectionID() {
						_ = adapter.Disconnect(context.Background())
						if oldHistory := m.histories[msg.adapter.ConnectionID()]; oldHistory != nil {
							_ = oldHistory.Close()
						}
						m.adapters[i] = msg.adapter
						m.channels.RemoveAdapter(msg.adapter.ConnectionID())
						replaced = true
						break
					}
				}
				if !replaced {
					m.adapters = append(m.adapters, msg.adapter)
				}
				m.statuses[msg.adapter.ConnectionID()] = msg.adapter.Status()
				m.channels.AddAdapter(msg.adapter)
				if msg.history != nil {
					m.histories[msg.adapter.ConnectionID()] = msg.history
				}
				m.status = "connecting " + string(msg.adapter.ConnectionID())
				m.reflow()
				return m, tea.Batch(func() tea.Msg {
					if err := msg.adapter.Connect(context.Background()); err != nil {
						return errorMsg{err}
					}
					return nil
				}, waitEvent(msg.adapter))
			}
		}
		m.reflow()
	case eventMsg:
		return m, m.applyEvent(msg.adapter, msg.event)
	case sendMsg:
		if err := m.channels.ApplyResult(msg.result); err != nil {
			m.status = err.Error()
		} else {
			m.status = string(msg.result.Status)
			if msg.result.DropReason != "" {
				m.status += ": " + msg.result.DropReason
			}
		}
	case reconnectMsg:
		delete(m.reconnecting, msg.id)
		if msg.err != nil {
			m.status = fmt.Sprintf("%s reconnect failed: %s", msg.id, msg.err)
		}
	case tea.KeyPressMsg:
		return m.key(msg.String())
	case tea.PasteMsg:
		if m.current == modeIntegrationSetup && m.setupEditing {
			m.pasteSetupValue(msg.Content)
			m.reflow()
		} else if m.current == modeNormal && !m.filterMode {
			m.composer += strings.NewReplacer("\r", "", "\n", " ").Replace(msg.Content)
			m.reflow()
		}
	case tea.MouseClickMsg:
		return m.click(msg.X, msg.Y)
	case tea.MouseWheelMsg:
		if msg.Y > 0 {
			m.scroll++
		} else if m.scroll > 0 {
			m.scroll--
		}
		m.reflow()
	}
	return m, nil
}

func waitEvent(adapter chat.Adapter) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-adapter.Events()
		if !ok {
			return nil
		}
		return eventMsg{adapter: adapter, event: event}
	}
}

func (m Model) applyEvent(adapter chat.Adapter, event chat.Event) tea.Cmd {
	switch event := event.(type) {
	case chat.MessageEvent:
		if !m.channels.AddMessage(m.burst.Offer(event.Message)) {
			break
		}
		if history := m.histories[event.Message.ConnectionID]; history != nil {
			if err := history.Append(event.Message); err != nil {
				m.status = "history: " + err.Error()
			}
		}
	case chat.MessageUpdateEvent:
		m.channels.UpdateMessage(event)
		if history := m.histories[event.Message.ConnectionID]; history != nil {
			if err := history.Append(event.Message); err != nil {
				m.status = "history: " + err.Error()
			}
		}
	case chat.StatusEvent:
		m.statuses[event.Status.ConnectionID] = event.Status
		m.status = fmt.Sprintf("%s: %s", event.Status.Platform, event.Status.State)
	}
	m.reflow()
	return waitEvent(adapter)
}

// AddHistoryMessage restores a persisted message before live events arrive.
func (m *Model) AddHistoryMessage(message chat.Message) {
	if m.channels == nil {
		return
	}
	m.channels.AddMessage(chat.RenderDecision{RenderNow: true, Message: message})
	m.reflow()
}

func (m Model) sendCommands(commands []channels.SendCommand) tea.Cmd {
	result := make([]tea.Cmd, 0, len(commands))
	for _, command := range commands {
		command := command
		result = append(result, func() tea.Msg {
			return sendMsg{result: command.Adapter.Send(context.Background(), command.Request)}
		})
	}
	return tea.Batch(result...)
}

func (m Model) nextTarget() {
	targets := m.channels.Targets()
	if len(targets) == 0 {
		return
	}
	current := m.channels.SelectedTarget()
	if current == channels.AllTargets {
		_ = m.channels.SelectTarget(targets[0].ConnectionID)
		return
	}
	for i, target := range targets {
		if target.ConnectionID == current {
			_ = m.channels.SelectTarget(targets[(i+1)%len(targets)].ConnectionID)
			return
		}
	}
}

func (m Model) reconnectSelected() tea.Cmd {
	target := m.channels.SelectedTarget()
	if target == channels.AllTargets {
		m.status = "select a connection before reconnecting"
		return nil
	}
	for _, adapter := range m.adapters {
		if adapter.ConnectionID() != target {
			continue
		}
		if m.reconnecting[target] {
			m.status = fmt.Sprintf("%s is already reconnecting", target)
			return nil
		}
		m.reconnecting[target] = true
		m.status = "reconnecting " + string(target)
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := adapter.Disconnect(ctx); err != nil {
				return reconnectMsg{id: target, err: err}
			}
			return reconnectMsg{id: target, err: adapter.Connect(ctx)}
		}
	}
	m.status = "selected target is unavailable"
	return nil
}

func (m Model) key(key string) (tea.Model, tea.Cmd) {
	if m.current == modeHelp {
		if matches(key, m.cfg.Keybinds.Back) {
			m.current = modeNormal
		}
		return m, nil
	}
	if m.current == modeTheme {
		return m.themeKey(key)
	}
	if m.current == modeHistory || m.current == modeUpdates {
		if matches(key, m.cfg.Keybinds.Back) {
			m.current = modeNormal
		}
		if m.current == modeUpdates && matches(key, m.cfg.Keybinds.Rollback) {
			return m, m.rollback()
		}
		return m, nil
	}
	if m.current == modeIntegrations {
		return m.integrationKey(key)
	}
	if m.current == modeIntegrationSetup {
		return m.integrationSetupKey(key)
	}
	if m.filterMode {
		switch key {
		case m.cfg.Keybinds.Back:
			m.filterMode, m.filter = false, ""
			m.channels.SetFilter("")
		case m.cfg.Keybinds.Confirm:
			m.filterMode = false
		case "backspace":
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
				m.channels.SetFilter(m.filter)
			}
		default:
			if key != "" && !strings.Contains(key, "+") {
				m.filter += key
				m.channels.SetFilter(m.filter)
			}
		}
		m.reflow()
		return m, nil
	}
	switch {
	case matches(key, m.cfg.Keybinds.Quit):
		return m, tea.Quit
	case matches(key, m.cfg.Keybinds.Help):
		m.current = modeHelp
	case matches(key, m.cfg.Keybinds.Theme):
		m.current = modeTheme
		m.themeCursor = m.selectedTheme()
	case matches(key, m.cfg.Keybinds.OpenConfig):
		return m, m.openConfig()
	case matches(key, m.cfg.Keybinds.Integrations):
		m.current = modeIntegrations
		m.integrationCursor = 0
	case matches(key, m.cfg.Keybinds.Copy):
		return m, copyText(m.commandPreview())
	case matches(key, m.cfg.Keybinds.Filter):
		m.filterMode, m.filter = true, ""
		m.channels.SetFilter("")
	case matches(key, m.cfg.Keybinds.NextTarget):
		m.nextTarget()
	case matches(key, m.cfg.Keybinds.Retry):
		command, err := m.channels.LatestRetryable()
		if err != nil {
			m.status = err.Error()
		} else {
			return m, m.sendCommands([]channels.SendCommand{command})
		}
	case matches(key, m.cfg.Keybinds.Reconnect):
		return m, m.reconnectSelected()
	case matches(key, m.cfg.Keybinds.ViewCombined):
		_ = m.channels.SetView(channels.ViewCombined)
	case matches(key, m.cfg.Keybinds.ViewTwitch):
		_ = m.channels.SetView(channels.ViewTwitch)
	case matches(key, m.cfg.Keybinds.ViewYouTube):
		_ = m.channels.SetView(channels.ViewYouTube)
	case matches(key, m.cfg.Keybinds.History):
		m.current = modeHistory
	case matches(key, m.cfg.Keybinds.Update):
		m.current = modeUpdates
		return m, m.checkUpdate()
	case matches(key, m.cfg.Keybinds.Down):
		m.cursor++
		m.ensureVisible()
	case matches(key, m.cfg.Keybinds.Up):
		m.cursor--
		m.ensureVisible()
	case matches(key, m.cfg.Keybinds.PageDown):
		m.cursor += m.visibleRows()
		m.ensureVisible()
	case matches(key, m.cfg.Keybinds.PageUp):
		m.cursor -= m.visibleRows()
		m.ensureVisible()
	case matches(key, m.cfg.Keybinds.First):
		m.cursor = 0
		m.ensureVisible()
	case matches(key, m.cfg.Keybinds.Last):
		m.cursor = len(m.items) - 1
		m.ensureVisible()
	case key == "backspace":
		if len(m.composer) > 0 {
			m.composer = m.composer[:len(m.composer)-1]
		}
	case matches(key, m.cfg.Keybinds.Confirm):
		m.channels.SetComposer(m.composer)
		commands, err := m.channels.Submit()
		if err != nil {
			m.status = err.Error()
		} else {
			m.composer = m.channels.Composer()
			return m, m.sendCommands(commands)
		}
	default:
		if key != "" && !strings.Contains(key, "+") {
			m.composer += key
		}
	}
	m.reflow()
	return m, nil
}

func (m Model) integrationKey(key string) (tea.Model, tea.Cmd) {
	switch {
	case matches(key, m.cfg.Keybinds.Back):
		m.current = modeNormal
	case matches(key, m.cfg.Keybinds.Down):
		m.integrationCursor = min(2, m.integrationCursor+1)
	case matches(key, m.cfg.Keybinds.Up):
		m.integrationCursor = max(0, m.integrationCursor-1)
	case matches(key, m.cfg.Keybinds.ProviderConsole) && m.integrationCursor < 2:
		return m, platform.OpenURL(integrationURL(m.integrationCursor))
	case matches(key, m.cfg.Keybinds.Confirm) && m.integrationCursor < 2:
		m.setupPlatform = []chat.Platform{chat.PlatformTwitch, chat.PlatformYouTube}[m.integrationCursor]
		m.setupField, m.setupValues = 0, [4]string{}
		m.setupValues[0] = nextConnectionID(m.cfg, m.setupPlatform)
		for _, connection := range m.cfg.Connections {
			if connection.Platform == m.setupPlatform {
				m.setupValues[0] = string(connection.ID)
				m.setupValues[1] = connection.Channel
				m.setupEnabled = connection.Enabled
				break
			}
		}
		if m.setupValues[0] == nextConnectionID(m.cfg, m.setupPlatform) {
			m.setupEnabled = true
		}
		m.setupValues[2] = m.cfg.Applications[m.setupPlatform].ClientID
		m.setupClientIDKept = m.setupValues[2] != ""
		m.setupSecretKept = false
		if credential, err := auth.NewCredentialStore(auth.OSKeyring{}).Load(m.setupPlatform, chat.ConnectionID(m.setupValues[0])); err == nil {
			m.setupValues[3] = credential.ClientSecret
			m.setupSecretKept = m.setupValues[3] != ""
		}
		m.setupEditing = false
		m.current = modeIntegrationSetup
	}
	m.reflow()
	return m, nil
}

func (m Model) integrationSetupKey(key string) (tea.Model, tea.Cmd) {
	if m.setupEditing {
		if matches(key, m.cfg.Keybinds.Back) {
			m.setupValues[m.setupField] = m.setupBeforeEdit
			m.setupEditing = false
			m.reflow()
			return m, nil
		}
		if matches(key, m.cfg.Keybinds.Confirm) {
			m.setupEditing = false
			m.reflow()
			return m, nil
		}
		if key == "backspace" {
			if m.setupField == 2 && m.setupClientIDKept {
				m.setupValues[2] = ""
				m.setupClientIDKept = false
			} else if m.setupField == 3 && m.setupSecretKept {
				m.setupValues[3] = ""
				m.setupSecretKept = false
			} else if len(m.setupValues[m.setupField]) > 0 {
				m.setupValues[m.setupField] = m.setupValues[m.setupField][:len(m.setupValues[m.setupField])-1]
			}
			m.reflow()
			return m, nil
		}
		if !m.isConfiguredControlKey(key) && key != "" && !strings.Contains(key, "+") {
			if m.setupField == 2 && m.setupClientIDKept {
				m.setupValues[2] = ""
				m.setupClientIDKept = false
			} else if m.setupField == 3 && m.setupSecretKept {
				m.setupValues[3] = ""
				m.setupSecretKept = false
			}
			m.setupValues[m.setupField] += key
		}
		m.reflow()
		return m, nil
	}
	if matches(key, m.cfg.Keybinds.Back) {
		m.current = modeIntegrations
		m.reflow()
		return m, nil
	}
	if matches(key, m.cfg.Keybinds.SaveIntegration) {
		return m, m.saveIntegration()
	}
	if matches(key, m.cfg.Keybinds.ToggleEnabled) {
		m.setupEnabled = !m.setupEnabled
		m.reflow()
		return m, nil
	}
	if matches(key, m.cfg.Keybinds.Confirm) {
		if m.setupField == len(m.setupValues) {
			return m, m.saveIntegration()
		}
		m.setupEditing = true
		m.setupBeforeEdit = m.setupValues[m.setupField]
		m.reflow()
		return m, nil
	}
	if matches(key, m.cfg.Keybinds.ProviderConsole) {
		cursor := 0
		if m.setupPlatform == chat.PlatformYouTube {
			cursor = 1
		}
		return m, platform.OpenURL(integrationURL(cursor))
	}
	if matches(key, m.cfg.Keybinds.Down) || matches(key, m.cfg.Keybinds.NextTarget) {
		m.setupField = min(len(m.setupValues), m.setupField+1)
	} else if matches(key, m.cfg.Keybinds.Up) {
		m.setupField = max(0, m.setupField-1)
	}
	m.reflow()
	return m, nil
}

func (m *Model) pasteSetupValue(content string) {
	content = strings.NewReplacer("\r", "", "\n", "").Replace(content)
	if m.setupField == 2 && m.setupClientIDKept {
		m.setupValues[2] = ""
		m.setupClientIDKept = false
	} else if m.setupField == 3 && m.setupSecretKept {
		m.setupValues[3] = ""
		m.setupSecretKept = false
	}
	m.setupValues[m.setupField] += content
}

func (m Model) saveIntegration() tea.Cmd {
	platformName, values := m.setupPlatform, m.setupValues
	return func() tea.Msg {
		connection := auth.ConnectionConfig{ID: chat.ConnectionID(values[0]), Platform: platformName, Channel: values[1], Enabled: m.setupEnabled}
		store := auth.NewCredentialStore(auth.OSKeyring{})
		credential := auth.Credential{ClientID: values[2], ClientSecret: values[3]}
		if existing, err := store.Load(platformName, connection.ID); err == nil {
			credential.AccessToken = existing.AccessToken
			credential.RefreshToken = existing.RefreshToken
			credential.ExpiresAt = existing.ExpiresAt
		}
		if err := store.Save(platformName, connection.ID, credential); err != nil {
			return integrationMsg{err: fmt.Errorf("save client secret: %w", err)}
		}
		persistedConnection := connection
		if m.connector != nil {
			// Keep an incomplete connection out of startup until OAuth and adapter setup succeed.
			persistedConnection.Enabled = false
		}
		if err := config.SaveIntegration(m.name, platformName, persistedConnection, values[2]); err != nil {
			return integrationMsg{err: err}
		}
		cfg, err := config.Load(m.name)
		if err != nil {
			return integrationMsg{err: err}
		}
		if !connection.Enabled {
			return integrationMsg{cfg: cfg, connectionID: connection.ID, enabled: connection.Enabled}
		}
		application := cfg.Applications[platformName]
		flow := auth.NewOAuthFlow(platformName, application, credential.ClientSecret)
		authenticated := false
		if credential.AccessToken != "" && (credential.ExpiresAt == 0 || time.Now().Unix() < credential.ExpiresAt) {
			authenticated = true
		}
		if !authenticated && credential.RefreshToken != "" {
			if refreshed, refreshErr := flow.Refresh(context.Background(), credential); refreshErr == nil {
				if saveErr := store.Save(platformName, connection.ID, refreshed); saveErr != nil {
					return integrationMsg{cfg: cfg, authErr: saveErr}
				}
				credential = refreshed
				authenticated = true
			}
		}
		if !authenticated {
			var authErr error
			credential, authErr = flow.Authorize(context.Background())
			if authErr != nil {
				return integrationMsg{cfg: cfg, authErr: authErr}
			}
			if err := store.Save(platformName, connection.ID, credential); err != nil {
				return integrationMsg{cfg: cfg, authErr: err}
			}
		}
		var adapter chat.Adapter
		var connectionHistory *chat.History
		var connectorErr error
		if m.connector != nil {
			adapter, connectionHistory, connectorErr = m.connector(context.Background(), cfg, connection, credential)
		}
		return integrationMsg{cfg: cfg, authenticated: true, adapter: adapter, history: connectionHistory, authErr: connectorErr, connectionID: connection.ID, enabled: connection.Enabled}
	}
}

func integrationURL(cursor int) string {
	if cursor == 0 {
		return "https://dev.twitch.tv/console/apps"
	}
	return "https://console.cloud.google.com/apis/credentials"
}

func nextConnectionID(cfg config.Config, platformName chat.Platform) string {
	prefix := string(platformName) + "-main"
	used := make(map[chat.ConnectionID]bool, len(cfg.Connections))
	for _, connection := range cfg.Connections {
		used[connection.ID] = true
	}
	if !used[chat.ConnectionID(prefix)] {
		return prefix
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", prefix, suffix)
		if !used[chat.ConnectionID(candidate)] {
			return candidate
		}
	}
}

func (m Model) themeKey(key string) (tea.Model, tea.Cmd) {
	switch {
	case matches(key, m.cfg.Keybinds.Back):
		m.current = modeNormal
	case matches(key, m.cfg.Keybinds.Down):
		m.themeCursor = min(len(m.themeNames)-1, m.themeCursor+1)
	case matches(key, m.cfg.Keybinds.Up):
		m.themeCursor = max(0, m.themeCursor-1)
	case matches(key, m.cfg.Keybinds.Confirm):
		if len(m.themeNames) > 0 {
			name := m.themeNames[m.themeCursor]
			if err := config.SetThemeName(m.name, name); err != nil {
				m.status = err.Error()
			} else {
				m.cfg.Themes.ThemeName = name
				m.styles = ui.NewStyles(config.ResolveTheme(m.cfg))
				m.status = "theme: " + name
			}
			m.current = modeNormal
		}
	}
	m.reflow()
	return m, nil
}

func (m Model) click(x, y int) (tea.Model, tea.Cmd) {
	for action, rect := range m.layout.Actions {
		if rect.Contains(x, y) {
			switch action {
			case "brand":
				return m, platform.OpenURL(ui.BrandURL)
			case "copy":
				return m, copyText(m.commandPreview())
			case "help":
				m.current = modeHelp
			case "theme":
				m.current = modeTheme
				m.themeCursor = m.selectedTheme()
			case "history":
				m.current = modeHistory
			case "update":
				m.current = modeUpdates
				return m, m.checkUpdate()
			}
		}
	}
	for index, rect := range m.layout.Items {
		if rect.Contains(x, y) {
			m.cursor = m.scroll + index
			m.ensureVisible()
			break
		}
	}
	m.reflow()
	return m, nil
}

func (m Model) reloadConfig() tea.Cmd {
	return func() tea.Msg { cfg, err := config.Load(m.name); return configMsg{cfg, err} }
}
func (m Model) openConfig() tea.Cmd {
	path, err := config.Path(m.name)
	if err != nil {
		return func() tea.Msg { return editorMsg{err: err} }
	}
	command, err := platform.EditorCommand(path, m.cfg.Editor)
	if err != nil {
		return func() tea.Msg { return editorMsg{err: err} }
	}
	return tea.ExecProcess(command, func(err error) tea.Msg { return editorMsg{err: err} })
}
func (m Model) loadHistory() tea.Cmd {
	return func() tea.Msg { entries, err := history.Load(m.cfg.History); return historyMsg{entries, err} }
}
func (m Model) checkUpdate() tea.Cmd {
	return func() tea.Msg { return updateMsg{update.Check(m.cfg.Updates, version.Commit)} }
}
func (m Model) rollback() tea.Cmd {
	return func() tea.Msg {
		if m.cfg.Updates.SourcePath == "" {
			return updateMsg{update.Result{Current: version.Commit, Error: fmt.Errorf("set updates.source_path before rollback")}}
		}
		if m.update.Latest == "" {
			return updateMsg{update.Result{Current: version.Commit, Error: fmt.Errorf("no update history is loaded")}}
		}
		err := update.InstallCommit(context.Background(), m.cfg.Updates.SourcePath, m.update.Latest)
		return updateMsg{update.Result{Current: version.Commit, Latest: m.update.Latest, Error: err}}
	}
}
func copyText(text string) tea.Cmd {
	return func() tea.Msg { return clipboardMsg{clipboard.WriteAll(text)} }
}

func (m Model) commandPreview() string {
	if m.composer == "" {
		return "# Type a message, then press Enter to send"
	}
	return m.composer
}
func matches(value, configured string) bool {
	return configured != "" && value == configured
}

func (m Model) isConfiguredControlKey(key string) bool {
	for _, configured := range []string{
		m.cfg.Keybinds.Up, m.cfg.Keybinds.Down, m.cfg.Keybinds.Left,
		m.cfg.Keybinds.Right, m.cfg.Keybinds.NextTarget, m.cfg.Keybinds.Back,
		m.cfg.Keybinds.Confirm,
	} {
		if matches(key, configured) {
			return true
		}
	}
	return false
}
func (m *Model) ensureVisible() {
	if len(m.items) == 0 {
		m.cursor, m.scroll = 0, 0
		return
	}
	m.cursor = clamp(m.cursor, 0, len(m.items)-1)
	rows := m.visibleRows()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+rows {
		m.scroll = m.cursor - rows + 1
	}
	m.scroll = clamp(m.scroll, 0, max(0, len(m.items)-rows))
}
func (m Model) visibleRows() int {
	footerRows := 1 + 1 + len(ui.Wrap("> "+m.composer, max(1, m.width))) + len(ui.Wrap(m.commandPreview(), max(1, m.width))) + 1
	if m.messageLengthStatus() != "" {
		footerRows++
	}
	if m.connectionStatus() != "" {
		footerRows++
	}
	if m.filterMode || m.filter != "" {
		footerRows++
	}
	if m.cfg.UI.ShowHints {
		footerRows++
	}
	return max(1, m.height-2-footerRows)
}
func (m *Model) reflow() {
	m.items = m.messageLines()
	m.ensureVisible()
	m.layout = buildLayout(m.width, m.height, m.styles, m.items, m.scroll, m.visibleRows(), m.cfg.UI.ShowLogo, m.cfg.UI.ShowHints)
}

func (m Model) messageLines() []string {
	if m.channels == nil {
		return nil
	}
	messages := m.channels.Messages()
	lines := make([]string, 0, len(messages))
	for _, message := range messages {
		marker := "[??]"
		if message.Platform == chat.PlatformTwitch {
			marker = "[TW]"
		} else if message.Platform == chat.PlatformYouTube {
			marker = "[YT]"
		}
		prefix := fmt.Sprintf("%s %s: ", marker, message.Author.DisplayName)
		wrapped := ui.Wrap(message.Text, max(1, m.width-lipgloss.Width(prefix)))
		for index, line := range wrapped {
			if index == 0 {
				lines = append(lines, prefix+line)
			} else {
				lines = append(lines, strings.Repeat(" ", lipgloss.Width(prefix))+line)
			}
		}
	}
	return lines
}
func (m Model) selectedTheme() int {
	for i, n := range m.themeNames {
		if n == m.cfg.Themes.ThemeName {
			return i
		}
	}
	return 0
}

func (m Model) View() tea.View {
	if m.width < 1 || m.height < 1 {
		return m.newView("")
	}
	m.reflow()
	if m.current == modeHelp {
		return m.newView(m.overlay("Help", []string{"Arrow keys or hjkl navigate.", "Enter sends, Esc goes back, PgUp/PgDown page.", "o edits TOML config; i configures integrations; T changes theme.", "y copies message; H opens history; U checks updates; q quits."}))
	}
	if m.current == modeTheme {
		names := make([]string, len(m.themeNames))
		for i, n := range m.themeNames {
			names[i] = n
			if i == m.themeCursor {
				names[i] = "> " + n + " (selected)"
			}
		}
		return m.newView(m.overlay("Themes", names))
	}
	if m.current == modeHistory {
		lines := []string{"Previous sessions and actions:"}
		for _, entry := range m.history {
			lines = append(lines, entry.String())
		}
		return m.newView(m.overlay("History", lines))
	}
	if m.current == modeUpdates {
		lines := []string{"Current: " + m.update.Current, "Latest: " + m.update.Latest}
		if m.update.Available {
			lines = append(lines, "Update available")
		} else {
			lines = append(lines, "No update available")
		}
		return m.newView(m.overlay("Updates", lines))
	}
	if m.current == modeIntegrations {
		items := []string{"Twitch", "YouTube", "Back"}
		for i := range items {
			if i == m.integrationCursor {
				items[i] = "> " + items[i]
			}
		}
		lines := []string{"Connect a chat provider without editing TOML:", m.cfg.Keybinds.Confirm + " selects, " + m.cfg.Keybinds.ProviderConsole + " opens the provider console, " + m.cfg.Keybinds.Back + " goes back.", ""}
		lines = append(lines, items...)
		return m.newView(m.overlay("Integrations", lines))
	}
	if m.current == modeIntegrationSetup {
		labels := []string{"Connection ID", "Channel name", "Client ID", "Client secret"}
		lines := []string{"Provider: " + string(m.setupPlatform), "Enter starts/stops editing; Tab advances; Esc cancels.", ""}
		if m.setupPlatform == chat.PlatformTwitch {
			lines = append(lines, "Twitch: press "+m.cfg.Keybinds.ProviderConsole+" to open the developer console and register the callback.")
		} else {
			lines = append(lines, "YouTube: press "+m.cfg.Keybinds.ProviderConsole+" to open Google Cloud; enable YouTube Data API v3.")
		}
		for i, label := range labels {
			value := m.setupValues[i]
			if i == 2 && m.setupClientIDKept {
				value = maskCredential(value)
			} else if i == 3 && value != "" {
				value = strings.Repeat("*", len([]rune(value)))
			}
			prefix := "  "
			if i == m.setupField {
				prefix = "> "
			}
			state := "[Enter to edit]"
			if i == m.setupField && m.setupEditing {
				state = "[editing]"
			}
			lines = append(lines, prefix+label+": "+value+" "+state)
		}
		savePrefix := "  "
		if m.setupField == len(m.setupValues) {
			savePrefix = "> "
		}
		enabledState := "disabled"
		if m.setupEnabled {
			enabledState = "enabled"
		}
		lines = append(lines, "Connection: "+enabledState+" ["+m.cfg.Keybinds.ToggleEnabled+" toggle]", savePrefix+"Save integration ["+m.cfg.Keybinds.SaveIntegration+"]", "", "Connection ID: a local name, such as twitch-main.", "Client ID: the public identifier from the provider console.", "Client secret: the private value paired with the client ID; never share it.", "", "The callback URL is "+auth.OAuthRedirectURL, m.cfg.Keybinds.Confirm+" starts/stops editing; "+m.cfg.Keybinds.NextTarget+" advances; "+m.cfg.Keybinds.SaveIntegration+" saves; "+m.cfg.Keybinds.ProviderConsole+" opens the provider console.")
		return m.newView(m.overlay("Set Up Integration", lines))
	}
	lines := []string{m.header(), ""}
	for i, rect := range m.layout.Items {
		_ = rect
		index := m.scroll + i
		prefix := "  "
		if index == m.cursor {
			prefix = "> "
		}
		lines = append(lines, prefix+m.items[index])
	}
	target := "*"
	if m.channels != nil {
		target = string(m.channels.SelectedTarget())
	}
	status := fmt.Sprintf("target: %s", target)
	if m.status != "" {
		status += " | " + m.status
	}
	lines = append(lines, "", m.styles.PreviewTitle.Render("Message"))
	for _, line := range ui.Wrap("> "+m.composer, max(1, m.width)) {
		lines = append(lines, m.styles.InputPrompt.Render(line))
	}
	for _, line := range ui.Wrap(m.commandPreview(), max(1, m.width)) {
		lines = append(lines, line)
	}
	lines = append(lines, m.styles.StatusBar.Render(status))
	if lengthStatus := m.messageLengthStatus(); lengthStatus != "" {
		lines = append(lines, m.styles.StatusBar.Render(lengthStatus))
	}
	if connectionStatus := m.connectionStatus(); connectionStatus != "" {
		lines = append(lines, m.styles.StatusBar.Render(connectionStatus))
	}
	if m.filterMode || m.filter != "" {
		lines = append(lines, m.styles.StatusBar.Render("filter: "+m.filter))
	}
	if m.cfg.UI.ShowHints {
		lines = append(lines, m.hints())
	}
	return m.newView(ui.JoinLines(lines, m.width, m.height))
}

func (m Model) messageLengthStatus() string {
	if m.channels == nil {
		return ""
	}
	limit, ok := m.channels.MessageLimit()
	if !ok {
		return ""
	}
	return fmt.Sprintf("message: %d/%d characters", chat.MessageLength(m.composer), limit)
}

func maskCredential(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	if len(runes) < 5 {
		return strings.Repeat("*", len(runes))
	}
	visible := max(1, len(runes)/10)
	if visible*2 >= len(runes) {
		return strings.Repeat("*", len(runes))
	}
	return string(runes[:visible]) + strings.Repeat("*", len(runes)-visible*2) + string(runes[len(runes)-visible:])
}

func (m Model) connectionStatus() string {
	status := ""
	if len(m.adapters) == 0 {
		for _, connection := range m.cfg.Connections {
			if !connection.Enabled {
				status = "connections: configured but disabled (" + string(connection.ID) + ")"
				break
			}
		}
		if status == "" {
			status = "connections: none configured"
		}
	} else {
		parts := make([]string, 0, len(m.adapters))
		for _, adapter := range m.adapters {
			state := m.statuses[adapter.ConnectionID()].State
			if m.reconnecting[adapter.ConnectionID()] {
				state = chat.StateReconnecting
			}
			parts = append(parts, fmt.Sprintf("%s=%s", adapter.ConnectionID(), state))
		}
		status = "connections: " + strings.Join(parts, " ")
	}
	if len(m.warnings) > 0 {
		status += " | warning: " + strings.Join(m.warnings, "; ")
	}
	return status
}
func (m Model) newView(content string) tea.View {
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}
func (m Model) header() string {
	title := m.styles.Header.Render(m.name)
	brand := ui.Brand(m.styles)
	gap := max(1, m.width-lipgloss.Width(title)-lipgloss.Width(brand))
	return title + strings.Repeat(" ", gap) + brand
}
func (m Model) hints() string {
	k := m.cfg.Keybinds
	return m.styles.HintKey.Render("["+k.Down+"/"+k.Up+"]") + " navigate  " + m.styles.HintKey.Render("["+k.Copy+"]") + " copy  " + m.styles.HintKey.Render("["+k.Help+"]") + " help  " + m.styles.HintKey.Render("["+k.Integrations+"]") + " integrations  " + m.styles.HintKey.Render("["+k.Quit+"]") + " quit"
}
func (m Model) overlay(title string, body []string) string {
	lines := []string{m.header(), m.styles.PreviewTitle.Render(title), ""}
	lines = append(lines, body...)
	lines = append(lines, "", m.hints())
	return ui.JoinLines(lines, m.width, m.height)
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func clamp(v, low, high int) int {
	if high < low {
		return low
	}
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
