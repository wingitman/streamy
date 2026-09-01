package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/wingitman/streamy/internal/app"
	"github.com/wingitman/streamy/internal/auth"
	"github.com/wingitman/streamy/internal/chat"
	"github.com/wingitman/streamy/internal/config"
	"github.com/wingitman/streamy/internal/twitch"
	"github.com/wingitman/streamy/internal/youtube"
)

func integrationConnector(configPath string) app.IntegrationConnector {
	return func(ctx context.Context, cfg config.Config, connection auth.ConnectionConfig, credential auth.Credential) (chat.Adapter, *chat.History, error) {
		if connection.Platform != chat.PlatformTwitch {
			return nil, nil, fmt.Errorf("automatic %s connection requires live_chat_id", connection.Platform)
		}
		broadcasterID, userID, err := resolveTwitchIDs(ctx, credential.ClientID, credential.AccessToken, connection.Channel)
		if err != nil {
			return nil, nil, err
		}
		connection.BroadcasterID, connection.UserID = broadcasterID, userID
		connection.Enabled = true
		if err := config.SaveIntegration("streamy", connection.Platform, connection, cfg.Applications[connection.Platform].ClientID); err != nil {
			return nil, nil, fmt.Errorf("save resolved connection metadata: %w", err)
		}
		clientID := credential.ClientID
		if clientID == "" {
			clientID = cfg.Applications[connection.Platform].ClientID
		}
		adapter, err := twitch.New(twitch.Config{ConnectionID: connection.ID, Channel: connection.Channel, BroadcasterID: broadcasterID, UserID: userID, ClientID: clientID, AccessToken: credential.AccessToken})
		if err != nil {
			return nil, nil, err
		}
		settings := chat.DefaultHistoryConfig()
		settings.Directory = filepath.Join(filepath.Dir(configPath), "history")
		settings.Path = cfg.History.File
		if cfg.History.MaxEntries > 0 {
			settings.MaxMessages = cfg.History.MaxEntries
		}
		settings.SessionID = "default"
		settings.ConnectionID = connection.ID
		settings.Platform = connection.Platform
		history, err := chat.NewHistory(settings)
		if err != nil {
			return nil, nil, err
		}
		return adapter, history, nil
	}
}

func resolveTwitchIDs(ctx context.Context, clientID, accessToken, channel string) (string, string, error) {
	userID, err := resolveTwitchUserID(ctx, accessToken)
	if err != nil {
		return "", "", err
	}
	requestURL := "https://api.twitch.tv/helix/users?login=" + url.QueryEscape(channel)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", "", err
	}
	request.Header.Set("Client-ID", clientID)
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", "", fmt.Errorf("resolve Twitch channel: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return "", "", fmt.Errorf("resolve Twitch channel: HTTP %s", response.Status)
	}
	var users struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&users); err != nil {
		return "", "", fmt.Errorf("decode Twitch channel: %w", err)
	}
	if len(users.Data) == 0 || users.Data[0].ID == "" {
		return "", "", fmt.Errorf("Twitch channel %q was not found", channel)
	}
	return users.Data[0].ID, userID, nil
}

func resolveTwitchUserID(ctx context.Context, accessToken string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://id.twitch.tv/oauth2/validate", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("validate Twitch token: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		return "", fmt.Errorf("validate Twitch token: HTTP %s", response.Status)
	}
	var result struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode Twitch token: %w", err)
	}
	if result.UserID == "" {
		return "", fmt.Errorf("Twitch token has no user ID")
	}
	return result.UserID, nil
}

func loadConfig() (config.Config, string, error) {
	path, err := config.Path("streamy")
	if err != nil {
		return config.Config{}, "", err
	}
	cfg, err := config.Load("streamy")
	return cfg, filepath.Clean(path), err
}

func buildAdapters(cfg config.Config) ([]chat.Adapter, []string) {
	store := auth.NewCredentialStore(auth.OSKeyring{})
	adapters := make([]chat.Adapter, 0, len(cfg.Connections))
	warnings := make([]string, 0)
	for _, connection := range cfg.Connections {
		if !connection.Enabled {
			continue
		}
		credential, err := store.Load(connection.Platform, connection.ID)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s unavailable: load credentials (run --login %s --connection %s): %v", connection.ID, connection.Platform, connection.ID, err))
			continue
		}
		if credential.AccessToken == "" && credential.RefreshToken != "" {
			application := cfg.Applications[connection.Platform]
			flow := auth.NewOAuthFlow(connection.Platform, application, credential.ClientSecret)
			credential, err = flow.Refresh(context.Background(), credential)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s unavailable: refresh credentials: %v", connection.ID, err))
				continue
			}
			if err := store.Save(connection.Platform, connection.ID, credential); err != nil {
				warnings = append(warnings, fmt.Sprintf("%s unavailable: save refreshed credentials: %v", connection.ID, err))
				continue
			}
		}
		if credential.AccessToken == "" {
			warnings = append(warnings, fmt.Sprintf("%s unavailable: no access token (run --login %s --connection %s)", connection.ID, connection.Platform, connection.ID))
			continue
		}
		clientID := credential.ClientID
		if clientID == "" {
			clientID = cfg.Applications[connection.Platform].ClientID
		}
		if clientID == "" {
			warnings = append(warnings, fmt.Sprintf("%s unavailable: no OAuth client_id in the keyring or config", connection.ID))
			continue
		}
		switch connection.Platform {
		case chat.PlatformTwitch:
			adapter, err := twitch.New(twitch.Config{ConnectionID: connection.ID, Channel: connection.Channel, BroadcasterID: connection.BroadcasterID, UserID: connection.UserID, ClientID: clientID, AccessToken: credential.AccessToken})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s unavailable: %v", connection.ID, err))
				continue
			}
			adapters = append(adapters, adapter)
		case chat.PlatformYouTube:
			adapter, err := youtube.New(youtube.Config{ConnectionID: connection.ID, LiveChatID: connection.LiveChatID, ClientID: clientID, AccessToken: credential.AccessToken})
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("%s unavailable: %v", connection.ID, err))
				continue
			}
			adapters = append(adapters, adapter)
		}
	}
	return adapters, warnings
}

func openHistories(cfg config.Config, configPath string, adapters []chat.Adapter) (map[chat.ConnectionID]*chat.History, error) {
	directory := filepath.Join(filepath.Dir(configPath), "history")
	histories := make(map[chat.ConnectionID]*chat.History, len(adapters))
	if cfg.History.Enabled && cfg.History.File != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.History.File), 0o700); err != nil {
			return nil, fmt.Errorf("create chat history directory: %w", err)
		}
		if len(adapters) == 0 {
			settings := chat.DefaultHistoryConfig()
			settings.Directory = filepath.Dir(cfg.History.File)
			settings.Path = cfg.History.File
			settings.SessionID = "default"
			settings.ConnectionID = "all"
			settings.Platform = "all"
			if cfg.History.MaxEntries > 0 {
				settings.MaxMessages = cfg.History.MaxEntries
			}
			history, err := chat.NewHistory(settings)
			if err != nil {
				return nil, err
			}
			_ = history.Close()
			return histories, nil
		}
		settings := chat.DefaultHistoryConfig()
		settings.Directory = filepath.Dir(cfg.History.File)
		settings.Path = cfg.History.File
		settings.SessionID = "default"
		settings.ConnectionID = adapters[0].ConnectionID()
		settings.Platform = adapters[0].Platform()
		if cfg.History.MaxEntries > 0 {
			settings.MaxMessages = cfg.History.MaxEntries
		}
		shared, err := chat.NewHistory(settings)
		if err != nil {
			return nil, err
		}
		for _, adapter := range adapters {
			histories[adapter.ConnectionID()] = shared
		}
		return histories, nil
	}
	for _, adapter := range adapters {
		settings := chat.DefaultHistoryConfig()
		settings.Directory = directory
		settings.SessionID = "default"
		settings.ConnectionID = adapter.ConnectionID()
		settings.Platform = adapter.Platform()
		history, err := chat.NewHistory(settings)
		if err != nil {
			closeHistories(histories)
			return nil, fmt.Errorf("open %s history: %w", adapter.ConnectionID(), err)
		}
		histories[adapter.ConnectionID()] = history
	}
	return histories, nil
}

func closeHistories(histories map[chat.ConnectionID]*chat.History) {
	for _, history := range histories {
		_ = history.Close()
	}
}

func disconnectAdapters(adapters []chat.Adapter) {
	for _, adapter := range adapters {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = adapter.Disconnect(ctx)
		cancel()
	}
}

func login(platform chat.Platform, connectionID chat.ConnectionID) error {
	cfg, path, err := loadConfig()
	if err != nil {
		return fmt.Errorf("load config %s: %w", path, err)
	}
	application, ok := cfg.Applications[platform]
	if !ok || application.ClientID == "" {
		return fmt.Errorf("%s OAuth client_id is not configured in %s", platform, path)
	}
	for _, connection := range cfg.Connections {
		if connection.ID == connectionID && connection.Platform == platform {
			store := auth.NewCredentialStore(auth.OSKeyring{})
			credential, err := store.Load(platform, connectionID)
			if err != nil {
				return fmt.Errorf("load %s client secret from keyring (store it before login): %w", connectionID, err)
			}
			flow := auth.NewOAuthFlow(platform, application, credential.ClientSecret)
			credential, err = flow.Authorize(context.Background())
			if err != nil {
				return err
			}
			return store.Save(platform, connectionID, credential)
		}
	}
	return fmt.Errorf("connection %s for %s is not configured in %s", connectionID, platform, path)
}
