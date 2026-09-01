package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/wingitman/streamy/internal/chat"
)

const (
	KeyringService       = "streamy"
	OAuthRedirectURL     = "http://127.0.0.1:43821/oauth/callback"
	TwitchAuthorization  = "https://id.twitch.tv/oauth2/authorize"
	YouTubeAuthorization = "https://accounts.google.com/o/oauth2/v2/auth"
)

type ConnectionConfig struct {
	ID       chat.ConnectionID `toml:"id"`
	Platform chat.Platform     `toml:"platform"`
	Channel  string            `toml:"channel"`
	Enabled  bool              `toml:"enabled"`
}

// OAuthApplication contains only values safe to keep in TOML. The client
// secret and user tokens belong in CredentialStore.
type OAuthApplication struct {
	ClientID    string   `toml:"client_id"`
	RedirectURL string   `toml:"redirect_url"`
	Scopes      []string `toml:"scopes"`
}

type Config struct {
	Connections  []ConnectionConfig                 `toml:"connections"`
	Applications map[chat.Platform]OAuthApplication `toml:"applications"`
}

func (c Config) Validate() error {
	seen := make(map[chat.ConnectionID]struct{}, len(c.Connections))
	for _, connection := range c.Connections {
		if connection.ID == "" || strings.TrimSpace(connection.Channel) == "" {
			return errors.New("each connection needs an id and channel")
		}
		if connection.Platform != chat.PlatformTwitch && connection.Platform != chat.PlatformYouTube {
			return fmt.Errorf("unsupported connection platform %q", connection.Platform)
		}
		if _, ok := seen[connection.ID]; ok {
			return fmt.Errorf("duplicate connection id %q", connection.ID)
		}
		seen[connection.ID] = struct{}{}
	}
	for platform, application := range c.Applications {
		if application.ClientID == "" {
			return fmt.Errorf("%s OAuth client id is required", platform)
		}
		if application.RedirectURL != OAuthRedirectURL {
			return fmt.Errorf("%s OAuth redirect must be %q", platform, OAuthRedirectURL)
		}
	}
	return nil
}

func DefaultOAuthApplication(platform chat.Platform) OAuthApplication {
	switch platform {
	case chat.PlatformTwitch:
		return OAuthApplication{
			RedirectURL: OAuthRedirectURL,
			Scopes:      []string{"user:read:chat", "user:write:chat"},
		}
	case chat.PlatformYouTube:
		return OAuthApplication{
			RedirectURL: OAuthRedirectURL,
			Scopes:      []string{"https://www.googleapis.com/auth/youtube.force-ssl"},
		}
	default:
		return OAuthApplication{RedirectURL: OAuthRedirectURL}
	}
}

type Credential struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

func (c Credential) Validate() error {
	if c.ClientID == "" || c.ClientSecret == "" {
		return errors.New("OAuth client credentials are required")
	}
	return nil
}

type SecretStore interface {
	Get(service, user string) (string, error)
	Set(service, user, secret string) error
	Delete(service, user string) error
}

type CredentialStore struct{ secrets SecretStore }

func NewCredentialStore(secrets SecretStore) *CredentialStore {
	return &CredentialStore{secrets: secrets}
}

func (s *CredentialStore) Load(platform chat.Platform, connectionID chat.ConnectionID) (Credential, error) {
	if s == nil || s.secrets == nil {
		return Credential{}, errors.New("credential store is not configured")
	}
	value, err := s.secrets.Get(KeyringService, credentialKey(platform, connectionID))
	if err != nil {
		return Credential{}, err
	}
	var credential Credential
	if err := json.Unmarshal([]byte(value), &credential); err != nil {
		return Credential{}, fmt.Errorf("decode stored credentials: %w", err)
	}
	return credential, nil
}

func (s *CredentialStore) Save(platform chat.Platform, connectionID chat.ConnectionID, credential Credential) error {
	if s == nil || s.secrets == nil {
		return errors.New("credential store is not configured")
	}
	if err := credential.Validate(); err != nil {
		return err
	}
	value, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("encode credentials: %w", err)
	}
	return s.secrets.Set(KeyringService, credentialKey(platform, connectionID), string(value))
}

func (s *CredentialStore) Delete(platform chat.Platform, connectionID chat.ConnectionID) error {
	if s == nil || s.secrets == nil {
		return errors.New("credential store is not configured")
	}
	return s.secrets.Delete(KeyringService, credentialKey(platform, connectionID))
}

func credentialKey(platform chat.Platform, connectionID chat.ConnectionID) string {
	return string(platform) + "/" + string(connectionID)
}
