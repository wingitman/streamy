package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wingitman/streamy/internal/auth"
	"github.com/wingitman/streamy/internal/chat"
)

func TestDefaultsIncludeCompleteInteractionContract(t *testing.T) {
	cfg := Default()
	for name, value := range map[string]string{"up": cfg.Keybinds.Up, "down": cfg.Keybinds.Down, "back": cfg.Keybinds.Back, "page_up": cfg.Keybinds.PageUp, "page_down": cfg.Keybinds.PageDown, "copy": cfg.Keybinds.Copy, "history": cfg.Keybinds.History, "update": cfg.Keybinds.Update, "rollback": cfg.Keybinds.Rollback, "integrations": cfg.Keybinds.Integrations, "filter": cfg.Keybinds.Filter, "next_target": cfg.Keybinds.NextTarget, "retry": cfg.Keybinds.Retry, "reconnect": cfg.Keybinds.Reconnect, "view_combined": cfg.Keybinds.ViewCombined, "view_twitch": cfg.Keybinds.ViewTwitch, "view_youtube": cfg.Keybinds.ViewYouTube, "provider_console": cfg.Keybinds.ProviderConsole, "save_integration": cfg.Keybinds.SaveIntegration} {
		if value == "" {
			t.Errorf("missing default binding %s", name)
		}
	}
	if cfg.History.MaxEntries < 1 || cfg.Updates.Repository == "" {
		t.Fatal("history/update defaults are incomplete")
	}
	if cfg.Keybinds.Quit != "esc" || cfg.Keybinds.Help != "ctrl+h" || cfg.Keybinds.Copy != "ctrl+y" {
		t.Fatalf("global key defaults = %#v", cfg.Keybinds)
	}
}

func TestMigrateLegacyKeybindDefaultsPreservesCustomBindings(t *testing.T) {
	content := "[keybinds]\nquit = \"q\"\nhelp = \"?\"\ncopy = \"my-copy\"\n"
	got := migrateLegacyKeybindDefaults(content)
	if !strings.Contains(got, `quit = "esc"`) || !strings.Contains(got, `help = "ctrl+h"`) {
		t.Fatalf("legacy defaults were not migrated: %s", got)
	}
	if !strings.Contains(got, `copy = "my-copy"`) {
		t.Fatalf("custom binding was changed: %s", got)
	}
}

func TestSaveIntegrationWritesProviderAndConnection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	err := SaveIntegration("streamy", chat.PlatformTwitch, auth.ConnectionConfig{
		ID: "twitch-main", Platform: chat.PlatformTwitch, Channel: "channel",
	}, "client-id")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("streamy")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Applications[chat.PlatformTwitch].ClientID != "client-id" || len(cfg.Connections) != 1 {
		t.Fatalf("saved integration = %#v", cfg)
	}
	data, err := os.ReadFile(filepath.Join(root, "delbysoft", "streamy.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), auth.OAuthRedirectURL) {
		t.Fatalf("saved config does not contain callback URL: %s", data)
	}
	if err := SaveIntegration("streamy", chat.PlatformTwitch, auth.ConnectionConfig{
		ID: "twitch-main", Platform: chat.PlatformTwitch, Channel: "updated-channel",
	}, "updated-client"); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load("streamy")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Connections) != 1 || cfg.Connections[0].Channel != "updated-channel" {
		t.Fatalf("existing integration was not updated: %#v", cfg.Connections)
	}
}

func TestLoadPreservesStreamyProviderConfiguration(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	dir := filepath.Join(root, "delbysoft")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	contents := `[applications.twitch]
client_id = "twitch-client"
redirect_url = "http://localhost:43821/oauth/callback"
scopes = ["user:read:chat"]

[[connections]]
id = "twitch-main"
platform = "twitch"
channel = "channel"
enabled = false
`
	if err := os.WriteFile(filepath.Join(dir, "streamy.toml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("streamy")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Applications["twitch"].ClientID != "twitch-client" || len(cfg.Connections) != 1 {
		t.Fatalf("provider config was not preserved: %#v", cfg)
	}
}
