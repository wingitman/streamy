package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/wingitman/streamy/internal/auth"
	"github.com/wingitman/streamy/internal/chat"
)

// Config is the root configuration for a generated Delbysoft TUI.
type Config struct {
	auth.Config
	Editor   string   `toml:"editor"`
	UI       UI       `toml:"ui"`
	Keybinds Keybinds `toml:"keybinds"`
	History  History  `toml:"history"`
	Updates  Updates  `toml:"updates"`
	Themes   Themes   `toml:"themes"`
}

// UI holds display preferences that are not part of the theme.
type UI struct {
	ShowHints bool `toml:"show_hints"`
	ShowLogo  bool `toml:"show_logo"`
}

// Keybinds holds configurable key mappings. Values use BubbleTea key names
// ("up", "down", "enter", "pgup", ...) or single characters like "q", "k".
type Keybinds struct {
	Up              string `toml:"up"`
	Down            string `toml:"down"`
	Confirm         string `toml:"confirm"`
	Quit            string `toml:"quit"`
	Help            string `toml:"help"`
	OpenConfig      string `toml:"open_config"`
	Theme           string `toml:"theme"`
	Left            string `toml:"left"`
	Right           string `toml:"right"`
	Back            string `toml:"back"`
	PageUp          string `toml:"page_up"`
	PageDown        string `toml:"page_down"`
	First           string `toml:"first"`
	Last            string `toml:"last"`
	Copy            string `toml:"copy"`
	History         string `toml:"history"`
	Update          string `toml:"update"`
	Rollback        string `toml:"rollback"`
	Integrations    string `toml:"integrations"`
	Filter          string `toml:"filter"`
	NextTarget      string `toml:"next_target"`
	Retry           string `toml:"retry"`
	Reconnect       string `toml:"reconnect"`
	ViewCombined    string `toml:"view_combined"`
	ViewTwitch      string `toml:"view_twitch"`
	ViewYouTube     string `toml:"view_youtube"`
	ProviderConsole string `toml:"provider_console"`
	SaveIntegration string `toml:"save_integration"`
	ToggleEnabled   string `toml:"toggle_enabled"`
}

type History struct {
	Enabled    bool   `toml:"enabled"`
	File       string `toml:"file"`
	MaxEntries int    `toml:"max_entries"`
}
type Updates struct {
	DisableChecks bool   `toml:"disable_checks"`
	Repository    string `toml:"repository"`
	SourcePath    string `toml:"source_path"`
	CurrentCommit string `toml:"current_commit"`
}

// Themes selects a shared Delbysoft theme and contains optional per-app
// overrides. The shared file is a collection of [themes.<name>] tables.
type Themes struct {
	ThemeName          string `toml:"theme_name"`
	ThemeFile          string `toml:"theme_file"`
	Foreground         string `toml:"foreground"`
	Background         string `toml:"background"`
	Primary            string `toml:"primary"`
	Accent             string `toml:"accent"`
	Muted              string `toml:"muted"`
	Error              string `toml:"error"`
	Success            string `toml:"success"`
	File               string `toml:"file"`
	Border             string `toml:"border"`
	SelectedBackground string `toml:"selected_background"`
	SelectedForeground string `toml:"selected_foreground"`
	HeaderBackground   string `toml:"header_background"`
	HintKey            string `toml:"hint_key"`
	ParentCrumb        string `toml:"parent_crumb"`
	RootDirectory      string `toml:"root_directory"`
	Clipboard          string `toml:"clipboard"`
	BrandPrimary       string `toml:"brand_primary"`
	BrandSecondary     string `toml:"brand_secondary"`
	Selector           string `toml:"selector"`
	ImageBackground    string `toml:"image_background"`
}

// ResolvedTheme describes the palette after applying the shared theme and any
// local overrides. Terminal means no explicit base colors should be used.
type ResolvedTheme struct {
	Colors   map[string]string
	Terminal bool
}

type themeFile struct {
	Themes map[string]Themes `toml:"themes"`
}

// keybindEntries is the single list of keybind TOML keys, used for writing
// defaults and migrating missing keys.
var keybindEntries = []struct{ key, comment string }{
	{"up", "move cursor up"},
	{"down", "move cursor down"},
	{"confirm", "confirm selection"},
	{"quit", "quit"},
	{"help", "show help"},
	{"open_config", "open config in editor"},
	{"theme", "open theme picker"},
	{"left", "move left or previous stage"}, {"right", "move right or next stage"},
	{"back", "cancel or go back"}, {"page_up", "page up"}, {"page_down", "page down"},
	{"first", "first item"}, {"last", "last item"}, {"copy", "copy current content"},
	{"history", "open history"}, {"update", "check for updates"}, {"rollback", "open rollback"},
	{"integrations", "open integrations"}, {"filter", "filter messages"}, {"next_target", "next connection"},
	{"retry", "retry message"}, {"reconnect", "reconnect connection"},
	{"view_combined", "show all providers"}, {"view_twitch", "show Twitch"}, {"view_youtube", "show YouTube"},
	{"provider_console", "open provider console"}, {"save_integration", "save integration"},
	{"toggle_enabled", "enable or disable connection"},
}

var themeEntries = []string{"theme_name", "theme_file"}

// Default returns a Config populated with all defaults.
func Default() Config {
	return Config{
		Config: auth.Config{Applications: map[chat.Platform]auth.OAuthApplication{
			chat.PlatformTwitch:  auth.DefaultOAuthApplication(chat.PlatformTwitch),
			chat.PlatformYouTube: auth.DefaultOAuthApplication(chat.PlatformYouTube),
		}},
		Editor: "",
		UI: UI{
			ShowHints: true,
			ShowLogo:  true,
		},
		Keybinds: Keybinds{
			Up:         "k",
			Down:       "j",
			Confirm:    "enter",
			Quit:       "esc",
			Help:       "ctrl+h",
			OpenConfig: "ctrl+o",
			Theme:      "ctrl+t",
			Left:       "h", Right: "l", Back: "esc", PageUp: "pgup", PageDown: "pgdown",
			First: "home", Last: "end", Copy: "ctrl+y", History: "ctrl+g", Update: "ctrl+u", Rollback: "ctrl+alt+r",
			Integrations: "ctrl+i", Filter: "ctrl+f", NextTarget: "ctrl+n", Retry: "ctrl+r", Reconnect: "ctrl+c",
			ViewCombined: "ctrl+1", ViewTwitch: "ctrl+2", ViewYouTube: "ctrl+3", ProviderConsole: "ctrl+b", SaveIntegration: "ctrl+s",
			ToggleEnabled: "ctrl+e",
		},
		History: History{Enabled: true, File: filepath.Join(ConfigDir(), "streamy-history.jsonl"), MaxEntries: 100},
		Updates: Updates{Repository: "https://github.com/wingitman/streamy"},
		Themes: Themes{
			ThemeName: "terminal",
			ThemeFile: filepath.Join(ConfigDir(), "themes.toml"),
		},
	}
}

// ConfigDir returns the platform-appropriate Delbysoft config directory.
func ConfigDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return ""
		}
		return filepath.Join(home, ".config", "delbysoft")
	}
	return filepath.Join(base, "delbysoft")
}

// Path returns the per-app config file path.
func Path(name string) (string, error) {
	dir := ConfigDir()
	if dir == "" {
		return "", errors.New("could not determine user config directory")
	}
	return filepath.Join(dir, name+".toml"), nil
}

// ThemeFilePath expands the configured shared theme file path.
func ThemeFilePath(cfg Config) string {
	if strings.TrimSpace(cfg.Themes.ThemeFile) == "" {
		return filepath.Join(ConfigDir(), "themes.toml")
	}
	path := strings.TrimSpace(cfg.Themes.ThemeFile)
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimLeft(path[1:], `/\`))
		}
	}
	return filepath.Clean(path)
}

// EnsureThemesFile creates the shared theme file if missing and appends any
// missing starter themes without overwriting existing ones.
func EnsureThemesFile(cfg Config) error {
	path := ThemeFilePath(cfg)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		updated := appendMissingStarterThemes(string(data))
		if updated == string(data) {
			return nil
		}
		return os.WriteFile(path, []byte(updated), 0644)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(defaultThemesTOML), 0644)
}

// ThemeNames returns terminal plus every named theme in the shared file.
func ThemeNames(cfg Config) ([]string, error) {
	var file themeFile
	if _, err := toml.DecodeFile(ThemeFilePath(cfg), &file); err != nil {
		return []string{"terminal"}, err
	}
	names := []string{"terminal"}
	for name := range file.Themes {
		names = append(names, name)
	}
	sort.Strings(names[1:])
	return names, nil
}

// SetThemeName updates only the selected theme in the app config file.
func SetThemeName(appName, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("theme name cannot be empty")
	}
	if err := EnsureConfig(appName, false); err != nil {
		return err
	}
	path, err := Path(appName)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := setSectionKey(string(data), "themes", "theme_name", quote(name))
	return os.WriteFile(path, []byte(content), 0644)
}

// SaveIntegration stores the non-secret setup values entered by the TUI.
// OAuth tokens and client secrets are deliberately handled by auth.CredentialStore.
func SaveIntegration(appName string, platform chat.Platform, connection auth.ConnectionConfig, clientID string) error {
	if strings.TrimSpace(clientID) == "" {
		return errors.New("client ID cannot be empty")
	}
	if connection.ID == "" || strings.TrimSpace(connection.Channel) == "" {
		return errors.New("connection ID and channel cannot be empty")
	}
	if platform != chat.PlatformTwitch && platform != chat.PlatformYouTube {
		return fmt.Errorf("unsupported connection platform %q", platform)
	}
	if connection.Platform != platform {
		return fmt.Errorf("connection platform %q does not match %q", connection.Platform, platform)
	}
	path, err := Path(appName)
	if err != nil {
		return err
	}
	if err := EnsureConfig(appName, false); err != nil {
		return err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var existing Config
	if _, err := toml.Decode(string(content), &existing); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	connectionExists := false
	for _, item := range existing.Connections {
		if item.ID == connection.ID {
			connectionExists = true
			break
		}
	}
	contentText := setSectionKey(string(content), "applications."+string(platform), "client_id", quote(clientID))
	contentText = setSectionKey(contentText, "applications."+string(platform), "redirect_url", quote(auth.OAuthRedirectURL))
	if connectionExists {
		contentText = setArraySectionKey(contentText, "connections", string(connection.ID), "channel", quote(connection.Channel))
		contentText = setArraySectionKey(contentText, "connections", string(connection.ID), "enabled", boolStr(connection.Enabled))
		contentText = setConnectionMetadata(contentText, connection)
		return os.WriteFile(path, []byte(contentText), 0644)
	}
	if !strings.HasSuffix(contentText, "\n") {
		contentText += "\n"
	}
	contentText += fmt.Sprintf("\n[[connections]]\nid = %s\nplatform = %s\nchannel = %s\nenabled = %s\n\n",
		quote(string(connection.ID)), quote(string(connection.Platform)), quote(connection.Channel), boolStr(connection.Enabled))
	contentText = setConnectionMetadata(contentText, connection)
	return os.WriteFile(path, []byte(contentText), 0644)
}

func setConnectionMetadata(content string, connection auth.ConnectionConfig) string {
	for key, value := range map[string]string{
		"broadcaster_id": quote(connection.BroadcasterID),
		"user_id":        quote(connection.UserID),
		"live_chat_id":   quote(connection.LiveChatID),
	} {
		if value != `""` {
			content = setArraySectionKey(content, "connections", string(connection.ID), key, value)
		}
	}
	return content
}

func setArraySectionKey(content, section, id, key, value string) string {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(content, newline)
	inArray, inTarget, found, targetIDLine := false, false, false, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[["+section+"]]" {
			inArray = true
			inTarget = false
			continue
		}
		if strings.HasPrefix(trimmed, "[") && !strings.HasPrefix(trimmed, "[[") {
			inArray = false
			inTarget = false
			continue
		}
		if inArray && !inTarget && strings.HasPrefix(trimmed, "id = ") && strings.Trim(strings.TrimPrefix(trimmed, "id = "), "\"") == id {
			inTarget = true
			targetIDLine = i
			continue
		}
		if !inTarget || !strings.HasPrefix(trimmed, key) {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + key + " = " + value
		found = true
		break
	}
	if !found && targetIDLine >= 0 {
		indent := lines[targetIDLine][:len(lines[targetIDLine])-len(strings.TrimLeft(lines[targetIDLine], " \t"))]
		lines = append(lines, "")
		copy(lines[targetIDLine+2:], lines[targetIDLine+1:])
		lines[targetIDLine+1] = indent + key + " = " + value
		return strings.Join(lines, newline)
	}
	if !found {
		return content
	}
	return strings.Join(lines, newline)
}

// ResolveTheme loads the selected shared theme and applies local overrides.
// A zero-value result means the caller should use its built-in fallback.
func ResolveTheme(cfg Config) ResolvedTheme {
	result := ResolvedTheme{Colors: map[string]string{}, Terminal: cfg.Themes.ThemeName == "terminal"}
	if !result.Terminal {
		var file themeFile
		if _, err := toml.DecodeFile(ThemeFilePath(cfg), &file); err != nil {
			return ResolvedTheme{}
		}
		selected, ok := file.Themes[cfg.Themes.ThemeName]
		if !ok {
			return ResolvedTheme{}
		}
		result.Colors = themeColors(selected)
		if len(result.Colors) == 0 {
			return ResolvedTheme{}
		}
	}
	for key, value := range themeColors(cfg.Themes) {
		result.Colors[key] = value
	}
	return result
}

// ValidateThemeFile checks the shared theme file without changing it.
func ValidateThemeFile(cfg Config) error {
	path := ThemeFilePath(cfg)
	var file themeFile
	if _, err := toml.DecodeFile(path, &file); err != nil {
		return fmt.Errorf("invalid themes file %q: %w", path, err)
	}
	if len(file.Themes) == 0 {
		return fmt.Errorf("themes file %q contains no [themes.<name>] entries", path)
	}
	return nil
}

func themeColors(t Themes) map[string]string {
	values := map[string]string{
		"foreground":          t.Foreground,
		"background":          t.Background,
		"primary":             t.Primary,
		"accent":              t.Accent,
		"muted":               t.Muted,
		"error":               t.Error,
		"success":             t.Success,
		"file":                t.File,
		"border":              t.Border,
		"selected_background": t.SelectedBackground,
		"selected_foreground": t.SelectedForeground,
		"header_background":   t.HeaderBackground,
		"hint_key":            t.HintKey,
		"parent_crumb":        t.ParentCrumb,
		"root_directory":      t.RootDirectory,
		"clipboard":           t.Clipboard,
		"brand_primary":       t.BrandPrimary,
		"brand_secondary":     t.BrandSecondary,
		"selector":            t.Selector,
		"image_background":    t.ImageBackground,
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		if validThemeColor(value) {
			result[key] = value
		}
	}
	return result
}

func validThemeColor(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if len(value) != 4 && len(value) != 7 {
		return false
	}
	if value[0] != '#' {
		return false
	}
	for _, c := range value[1:] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// Load reads the config file, creating it with defaults if it doesn't exist.
func Load(name string) (Config, error) {
	cfg := Default()
	path, err := Path(name)
	if err != nil {
		return cfg, err
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := EnsureConfig(name, false); err != nil {
			return cfg, err
		}
		_ = EnsureThemesFile(cfg)
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		_ = EnsureThemesFile(Default())
		return Default(), err
	}
	for platform, application := range cfg.Applications {
		if application.RedirectURL == auth.LegacyOAuthRedirectURL {
			application.RedirectURL = auth.OAuthRedirectURL
			cfg.Applications[platform] = application
		}
	}
	if err := cfg.Config.Validate(); err != nil {
		return Default(), err
	}

	applyKeybindDefaults(&cfg)
	if cfg.Themes.ThemeName == "" {
		cfg.Themes.ThemeName = "terminal"
	}
	if cfg.Themes.ThemeFile == "" {
		cfg.Themes.ThemeFile = filepath.Join(ConfigDir(), "themes.toml")
	}

	if data, readErr := os.ReadFile(path); readErr == nil {
		updated := migrateLegacyKeybindDefaults(ensureMissingConfigKeys(string(data)))
		if updated != string(data) {
			_ = os.WriteFile(path, []byte(updated), 0644)
		}
	}
	_ = EnsureThemesFile(cfg)
	return cfg, nil
}

// migrateLegacyKeybindDefaults updates values emitted by older Streamy
// templates while leaving other custom bindings untouched.
func migrateLegacyKeybindDefaults(content string) string {
	legacy := map[string]string{
		`quit = "q"`:        `quit = "esc"`,
		`help = "?"`:        `help = "ctrl+h"`,
		`open_config = "o"`: `open_config = "ctrl+o"`,
		`theme = "T"`:       `theme = "ctrl+t"`,
		`copy = "y"`:        `copy = "ctrl+y"`,
		`history = "H"`:     `history = "ctrl+g"`,
		`update = "U"`:      `update = "ctrl+u"`,
		`rollback = "R"`:    `rollback = "ctrl+alt+r"`,
	}
	for oldValue, newValue := range legacy {
		content = strings.ReplaceAll(content, oldValue, newValue)
	}
	return content
}

func applyKeybindDefaults(cfg *Config) {
	defaults := Default()
	d := defaults.Keybinds
	if cfg.Keybinds.Up == "" {
		cfg.Keybinds.Up = d.Up
	}
	if cfg.Keybinds.Down == "" {
		cfg.Keybinds.Down = d.Down
	}
	if cfg.Keybinds.Confirm == "" {
		cfg.Keybinds.Confirm = d.Confirm
	}
	if cfg.Keybinds.Quit == "" {
		cfg.Keybinds.Quit = d.Quit
	}
	if cfg.Keybinds.Help == "" {
		cfg.Keybinds.Help = d.Help
	}
	if cfg.Keybinds.OpenConfig == "" {
		cfg.Keybinds.OpenConfig = d.OpenConfig
	}
	if cfg.Keybinds.Theme == "" {
		cfg.Keybinds.Theme = d.Theme
	}
	if cfg.Keybinds.Left == "" {
		cfg.Keybinds.Left = d.Left
	}
	if cfg.Keybinds.Right == "" {
		cfg.Keybinds.Right = d.Right
	}
	if cfg.Keybinds.Back == "" {
		cfg.Keybinds.Back = d.Back
	}
	if cfg.Keybinds.PageUp == "" {
		cfg.Keybinds.PageUp = d.PageUp
	}
	if cfg.Keybinds.PageDown == "" {
		cfg.Keybinds.PageDown = d.PageDown
	}
	if cfg.Keybinds.First == "" {
		cfg.Keybinds.First = d.First
	}
	if cfg.Keybinds.Last == "" {
		cfg.Keybinds.Last = d.Last
	}
	if cfg.Keybinds.Copy == "" {
		cfg.Keybinds.Copy = d.Copy
	}
	if cfg.Keybinds.History == "" {
		cfg.Keybinds.History = d.History
	}
	if cfg.Keybinds.Update == "" {
		cfg.Keybinds.Update = d.Update
	}
	if cfg.Keybinds.Rollback == "" {
		cfg.Keybinds.Rollback = d.Rollback
	}
	if cfg.Keybinds.Integrations == "" {
		cfg.Keybinds.Integrations = d.Integrations
	}
	if cfg.Keybinds.Filter == "" {
		cfg.Keybinds.Filter = d.Filter
	}
	if cfg.Keybinds.NextTarget == "" {
		cfg.Keybinds.NextTarget = d.NextTarget
	}
	if cfg.Keybinds.Retry == "" {
		cfg.Keybinds.Retry = d.Retry
	}
	if cfg.Keybinds.Reconnect == "" {
		cfg.Keybinds.Reconnect = d.Reconnect
	}
	if cfg.Keybinds.ViewCombined == "" {
		cfg.Keybinds.ViewCombined = d.ViewCombined
	}
	if cfg.Keybinds.ViewTwitch == "" {
		cfg.Keybinds.ViewTwitch = d.ViewTwitch
	}
	if cfg.Keybinds.ViewYouTube == "" {
		cfg.Keybinds.ViewYouTube = d.ViewYouTube
	}
	if cfg.Keybinds.ProviderConsole == "" {
		cfg.Keybinds.ProviderConsole = d.ProviderConsole
	}
	if cfg.Keybinds.SaveIntegration == "" {
		cfg.Keybinds.SaveIntegration = d.SaveIntegration
	}
	if cfg.Keybinds.ToggleEnabled == "" {
		cfg.Keybinds.ToggleEnabled = d.ToggleEnabled
	}
	if cfg.History.File == "" {
		cfg.History.File = defaults.History.File
	}
	if cfg.History.MaxEntries < 1 {
		cfg.History.MaxEntries = defaults.History.MaxEntries
	}
	if cfg.Updates.Repository == "" {
		cfg.Updates.Repository = defaults.Updates.Repository
	}
}

// EnsureConfig creates the config if missing and adds missing known keys.
// When resetDefault is true it rewrites the full file to defaults.
func EnsureConfig(name string, resetDefault bool) error {
	path, err := Path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return err
	}
	if resetDefault {
		if err := os.WriteFile(path, []byte(Example(name)), 0644); err != nil {
			return err
		}
		return EnsureThemesFile(Default())
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(Example(name)), 0644); err != nil {
			return err
		}
		return EnsureThemesFile(Default())
	}
	if err != nil {
		return err
	}
	updated := ensureMissingConfigKeys(string(data))
	if updated != string(data) {
		if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
			return err
		}
	}
	return EnsureThemesFile(Default())
}

// Example returns the default config file content for the app.
func Example(name string) string {
	cfg := Default()
	return "# " + name + " configuration\n" +
		"# Leave editor empty to use VISUAL, EDITOR, or a platform default.\n" +
		"editor = " + quote(cfg.Editor) + "\n\n" +
		buildKeybindsSection(cfg) +
		"[ui]\n" +
		"show_hints = " + boolStr(cfg.UI.ShowHints) + "\n" +
		"show_logo = " + boolStr(cfg.UI.ShowLogo) + "\n\n" +
		"[history]\n" +
		"enabled = " + boolStr(cfg.History.Enabled) + "\n" +
		"file = " + quote(cfg.History.File) + "\n" +
		fmt.Sprintf("max_entries = %d\n\n", cfg.History.MaxEntries) +
		"[updates]\n" +
		"disable_checks = " + boolStr(cfg.Updates.DisableChecks) + "\n" +
		"repository = " + quote(cfg.Updates.Repository) + "\n" +
		"current_commit = " + quote(cfg.Updates.CurrentCommit) + "\n\n" +
		buildApplicationsSection(cfg) +
		buildThemesSection(cfg)
}

func buildApplicationsSection(cfg Config) string {
	return "[applications.twitch]\n" +
		"client_id = \"\"\n" +
		"redirect_url = \"" + auth.OAuthRedirectURL + "\"\n" +
		"scopes = [\"user:read:chat\", \"user:write:chat\"]\n\n" +
		"[applications.youtube]\n" +
		"client_id = \"\"\n" +
		"redirect_url = \"" + auth.OAuthRedirectURL + "\"\n" +
		"scopes = [\"https://www.googleapis.com/auth/youtube.force-ssl\"]\n\n" +
		"# Add one or more connections. Credentials are stored in the OS keyring.\n" +
		"# [[connections]]\n" +
		"# id = \"twitch-main\"\n" +
		"# platform = \"twitch\"\n" +
		"# channel = \"your-channel\"\n" +
		"# broadcaster_id = \"\"\n" +
		"# user_id = \"\"\n" +
		"# enabled = true\n\n"
}

func buildKeybindsSection(cfg Config) string {
	out := "[keybinds]\n"
	for _, e := range keybindEntries {
		out += e.key + " = " + quote(keybindValue(cfg, e.key)) + "   # " + e.comment + "\n"
	}
	return out + "\n"
}

func buildThemesSection(cfg Config) string {
	t := cfg.Themes
	return "[themes]\n" +
		"theme_name = " + quote(t.ThemeName) + "   # terminal, or a named theme from theme_file\n" +
		"theme_file = " + quote(t.ThemeFile) + "   # shared Delbysoft theme file\n" +
		"# Optional overrides applied after the selected theme.\n" +
		"# foreground = \"#FFFFFF\"\n" +
		"# background = \"#101522\"\n" +
		"# primary = \"#7C9EF0\"\n" +
		"# accent = \"#F0A47C\"\n" +
		"# muted = \"#666688\"\n" +
		"# error = \"#F07C7C\"\n" +
		"# success = \"#7CF09C\"\n" +
		"# file = \"#B0B0CC\"\n" +
		"# border = \"#444466\"\n" +
		"# selected_background = \"#cd0fc1\"\n" +
		"# selected_foreground = \"#EEEEFF\"\n" +
		"# header_background = \"#1A1A2E\"\n" +
		"# hint_key = \"#FFE66D\"\n" +
		"# parent_crumb = \"#3A3A5A\"\n" +
		"# root_directory = \"#555577\"\n" +
		"# clipboard = \"#F0E07C\"\n" +
		"# brand_primary = \"#FFFFFF\"\n" +
		"# brand_secondary = \"#5865F2\"\n" +
		"# selector = \"#FFFFFF\"\n" +
		"# image_background = \"#1A1A2E\"\n"
}

func keybindValue(cfg Config, key string) string {
	k := cfg.Keybinds
	switch key {
	case "up":
		return k.Up
	case "down":
		return k.Down
	case "confirm":
		return k.Confirm
	case "quit":
		return k.Quit
	case "help":
		return k.Help
	case "open_config":
		return k.OpenConfig
	case "theme":
		return k.Theme
	case "left":
		return k.Left
	case "right":
		return k.Right
	case "back":
		return k.Back
	case "page_up":
		return k.PageUp
	case "page_down":
		return k.PageDown
	case "first":
		return k.First
	case "last":
		return k.Last
	case "copy":
		return k.Copy
	case "history":
		return k.History
	case "update":
		return k.Update
	case "rollback":
		return k.Rollback
	case "integrations":
		return k.Integrations
	case "filter":
		return k.Filter
	case "next_target":
		return k.NextTarget
	case "retry":
		return k.Retry
	case "reconnect":
		return k.Reconnect
	case "view_combined":
		return k.ViewCombined
	case "view_twitch":
		return k.ViewTwitch
	case "view_youtube":
		return k.ViewYouTube
	case "provider_console":
		return k.ProviderConsole
	case "save_integration":
		return k.SaveIntegration
	case "toggle_enabled":
		return k.ToggleEnabled
	}
	return ""
}

func ensureMissingConfigKeys(content string) string {
	content = ensureSectionEntries(content, "keybinds", keybindDefaultLines())
	content = ensureSectionEntries(content, "ui", uiDefaultLines())
	content = ensureSectionEntries(content, "history", historyDefaultLines())
	content = ensureSectionEntries(content, "updates", updateDefaultLines())
	content = ensureSectionEntries(content, "themes", themeDefaultLines())
	return content
}

func keybindDefaultLines() map[string]string {
	cfg := Default()
	lines := map[string]string{}
	for _, e := range keybindEntries {
		lines[e.key] = e.key + " = " + quote(keybindValue(cfg, e.key))
	}
	return lines
}

func uiDefaultLines() map[string]string {
	d := Default().UI
	return map[string]string{
		"show_hints": "show_hints = " + boolStr(d.ShowHints),
		"show_logo":  "show_logo = " + boolStr(d.ShowLogo),
	}
}

func historyDefaultLines() map[string]string {
	d := Default().History
	return map[string]string{"enabled": "enabled = " + boolStr(d.Enabled), "file": "file = " + quote(d.File), "max_entries": fmt.Sprintf("max_entries = %d", d.MaxEntries)}
}

func updateDefaultLines() map[string]string {
	d := Default().Updates
	return map[string]string{"disable_checks": "disable_checks = " + boolStr(d.DisableChecks), "repository": "repository = " + quote(d.Repository), "source_path": "source_path = " + quote(d.SourcePath), "current_commit": "current_commit = " + quote(d.CurrentCommit)}
}

func themeDefaultLines() map[string]string {
	d := Default().Themes
	return map[string]string{
		"theme_name": "theme_name = " + quote(d.ThemeName),
		"theme_file": "theme_file = " + quote(d.ThemeFile),
	}
}

func ensureSectionEntries(content, section string, entries map[string]string) string {
	for _, key := range orderedKeys(entries) {
		if sectionContainsKey(content, section, key) {
			continue
		}
		content = insertSectionLine(content, section, entries[key])
	}
	return content
}

func sectionContainsKey(content, section, key string) bool {
	inSection := false
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == "["+section+"]"
			continue
		}
		if !inSection || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, key+"=") || strings.HasPrefix(trimmed, key+" ") {
			return true
		}
	}
	return false
}

func insertSectionLine(content, section, line string) string {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(content, newline)
	sectionHeader := "[" + section + "]"
	sectionIdx := -1
	insertIdx := len(lines)
	for i, text := range lines {
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if trimmed == sectionHeader {
				sectionIdx = i
				insertIdx = len(lines)
				continue
			}
			if sectionIdx >= 0 {
				insertIdx = i
				break
			}
		}
	}
	if sectionIdx < 0 {
		if strings.TrimSpace(content) != "" && !strings.HasSuffix(content, newline) {
			content += newline
		}
		return content + newline + sectionHeader + newline + line + newline
	}
	lines = append(lines[:insertIdx], append([]string{line}, lines[insertIdx:]...)...)
	return strings.Join(lines, newline)
}

func orderedKeys(entries map[string]string) []string {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func setSectionKey(content, section, key, value string) string {
	if !sectionContainsKey(content, section, key) {
		return insertSectionLine(content, section, key+" = "+value)
	}
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(content, newline)
	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inSection = trimmed == "["+section+"]"
			continue
		}
		if !inSection || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, key+"=") || strings.HasPrefix(trimmed, key+" ") {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			comment := ""
			if idx := strings.Index(line, "#"); idx >= 0 {
				comment = " " + strings.TrimSpace(line[idx:])
			}
			lines[i] = indent + key + " = " + value + comment
			break
		}
	}
	return strings.Join(lines, newline)
}

func appendMissingStarterThemes(content string) string {
	for _, name := range starterThemeNames {
		header := "[themes." + name + "]"
		if strings.Contains(content, header) {
			continue
		}
		block := starterThemeBlock(name)
		if block == "" {
			continue
		}
		if strings.TrimSpace(content) != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n" + block + "\n"
	}
	return content
}

func starterThemeBlock(name string) string {
	start := strings.Index(defaultThemesTOML, "[themes."+name+"]")
	if start < 0 {
		return ""
	}
	end := strings.Index(defaultThemesTOML[start:], "\n\n[themes.")
	if end < 0 {
		return strings.TrimSpace(defaultThemesTOML[start:])
	}
	return strings.TrimSpace(defaultThemesTOML[start : start+end])
}

func quote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

var starterThemeNames = []string{
	"ocean", "high_contrast", "redteam", "blueteam", "vim", "neovim",
	"monotone", "cyberpunk", "sands",
}

const defaultThemesTOML = `# Shared themes for Delbysoft terminal applications.
# Add themes as [themes.name] tables. Colors use #RGB or #RRGGBB values.
# Supported colors: foreground, background, primary, accent, muted, error,
# success, file, border, selected_background, selected_foreground,
# header_background, hint_key, parent_crumb, root_directory, clipboard,
# brand_primary, brand_secondary, selector, image_background.

[themes.ocean]
foreground = "#D7E3FF"
background = "#101522"
primary = "#7C9EF0"
accent = "#F0A47C"
muted = "#66708F"
error = "#F07C7C"
success = "#7CF09C"
file = "#B0B0CC"
border = "#35415F"
selected_background = "#3568B8"
selected_foreground = "#FFFFFF"
header_background = "#17213A"
hint_key = "#FFE66D"
parent_crumb = "#58627F"
root_directory = "#7D88A8"
clipboard = "#F0E07C"
brand_primary = "#FFFFFF"
brand_secondary = "#6F86FF"
selector = "#FFFFFF"
image_background = "#101522"

[themes.high_contrast]
foreground = "#FFFFFF"
background = "#000000"
primary = "#00FFFF"
accent = "#FFFF00"
muted = "#C0C0C0"
error = "#FF5555"
success = "#00FF00"
file = "#FFFFFF"
border = "#FFFFFF"
selected_background = "#FFFF00"
selected_foreground = "#000000"
header_background = "#000000"
hint_key = "#FFFF00"
parent_crumb = "#C0C0C0"
root_directory = "#FFFFFF"
clipboard = "#FFFF00"
brand_primary = "#FFFFFF"
brand_secondary = "#00FFFF"
selector = "#FFFF00"
image_background = "#000000"

[themes.redteam]
foreground = "#FFE8E8"
background = "#210B0B"
primary = "#FF6B6B"
accent = "#FFB86B"
muted = "#A97878"
error = "#FF3333"
success = "#8BE28B"
file = "#F2CACA"
border = "#713333"
selected_background = "#9E2020"
selected_foreground = "#FFFFFF"
header_background = "#3A1010"
hint_key = "#FFD166"
parent_crumb = "#805555"
root_directory = "#C88A8A"
clipboard = "#FFD166"
brand_primary = "#FFFFFF"
brand_secondary = "#FF4D4D"
selector = "#FFFFFF"
image_background = "#210B0B"

[themes.blueteam]
foreground = "#E7F1FF"
background = "#081525"
primary = "#69A7FF"
accent = "#72E0D1"
muted = "#6D86A5"
error = "#FF7B8B"
success = "#7DDEB3"
file = "#C4D8F2"
border = "#294C75"
selected_background = "#1557A5"
selected_foreground = "#FFFFFF"
header_background = "#0D223D"
hint_key = "#F4D35E"
parent_crumb = "#4D6888"
root_directory = "#8BA9CB"
clipboard = "#F4D35E"
brand_primary = "#FFFFFF"
brand_secondary = "#69A7FF"
selector = "#FFFFFF"
image_background = "#081525"

[themes.vim]
foreground = "#D7D7AF"
background = "#1C1C1C"
primary = "#87AF87"
accent = "#D7AF5F"
muted = "#808080"
error = "#AF5F5F"
success = "#87AF87"
file = "#D7D7AF"
border = "#5F5F5F"
selected_background = "#5F5F00"
selected_foreground = "#FFFFAF"
header_background = "#262626"
hint_key = "#FFFF87"
parent_crumb = "#5F875F"
root_directory = "#AFAF87"
clipboard = "#D7AF5F"
brand_primary = "#FFFFFF"
brand_secondary = "#87AF87"
selector = "#FFFFAF"
image_background = "#1C1C1C"

[themes.neovim]
foreground = "#C8D3F5"
background = "#1B1D2B"
primary = "#82AAFF"
accent = "#FFC777"
muted = "#828BB8"
error = "#FF757F"
success = "#C3E88D"
file = "#C8D3F5"
border = "#444A73"
selected_background = "#394B70"
selected_foreground = "#FFFFFF"
header_background = "#222436"
hint_key = "#FFCB6B"
parent_crumb = "#545C8C"
root_directory = "#A9B8E8"
clipboard = "#C3E88D"
brand_primary = "#FFFFFF"
brand_secondary = "#82AAFF"
selector = "#FFFFFF"
image_background = "#1B1D2B"

[themes.monotone]
foreground = "#D0D0D0"
background = "#202020"
primary = "#E0E0E0"
accent = "#FFFFFF"
muted = "#808080"
error = "#B0B0B0"
success = "#D8D8D8"
file = "#C0C0C0"
border = "#606060"
selected_background = "#D0D0D0"
selected_foreground = "#101010"
header_background = "#303030"
hint_key = "#FFFFFF"
parent_crumb = "#707070"
root_directory = "#A0A0A0"
clipboard = "#FFFFFF"
brand_primary = "#FFFFFF"
brand_secondary = "#A0A0A0"
selector = "#FFFFFF"
image_background = "#202020"

[themes.cyberpunk]
foreground = "#F4E8FF"
background = "#170D24"
primary = "#00E5FF"
accent = "#FFEA00"
muted = "#9B75B5"
error = "#FF3864"
success = "#39FF14"
file = "#E6CFFF"
border = "#7A2F9B"
selected_background = "#D100A8"
selected_foreground = "#FFFFFF"
header_background = "#28113C"
hint_key = "#FFEA00"
parent_crumb = "#754D91"
root_directory = "#C68AF0"
clipboard = "#FFEA00"
brand_primary = "#FFFFFF"
brand_secondary = "#00E5FF"
selector = "#FFFFFF"
image_background = "#170D24"

[themes.sands]
foreground = "#F3E7CE"
background = "#282016"
primary = "#E4B96A"
accent = "#F2D06B"
muted = "#9F8B6D"
error = "#D9795B"
success = "#A8B875"
file = "#E8D6B5"
border = "#6D583C"
selected_background = "#A66A2C"
selected_foreground = "#FFF4D6"
header_background = "#382A1B"
hint_key = "#F2D06B"
parent_crumb = "#806B4E"
root_directory = "#CBAE7A"
clipboard = "#F2D06B"
brand_primary = "#FFF4D6"
brand_secondary = "#E4B96A"
selector = "#FFF4D6"
image_background = "#282016"
`
