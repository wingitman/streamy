package auth

import (
	"errors"
	"testing"

	"github.com/wingitman/streamy/internal/chat"
)

type memorySecrets struct {
	service string
	user    string
	value   string
}

func (m *memorySecrets) Get(service, user string) (string, error) {
	if m.value == "" {
		return "", errors.New("not found")
	}
	if service != m.service || user != m.user {
		return "", errors.New("wrong key")
	}
	return m.value, nil
}
func (m *memorySecrets) Set(service, user, value string) error {
	m.service, m.user, m.value = service, user, value
	return nil
}
func (m *memorySecrets) Delete(service, user string) error {
	m.service, m.user, m.value = service, user, ""
	return nil
}

func TestConfigValidationRejectsDuplicateConnectionsAndSecretsInConfig(t *testing.T) {
	config := Config{
		Connections: []ConnectionConfig{
			{ID: "main", Platform: chat.PlatformTwitch, Channel: "streamer"},
			{ID: "main", Platform: chat.PlatformYouTube, Channel: "streamer"},
		},
	}
	if err := config.Validate(); err == nil {
		t.Fatal("duplicate connection IDs were accepted")
	}
	application := DefaultOAuthApplication(chat.PlatformTwitch)
	if application.ClientID != "" || application.RedirectURL != OAuthRedirectURL {
		t.Fatalf("default application contains an unsafe or incorrect value: %#v", application)
	}
}

func TestCredentialStoreUsesNamespacedKeyringAndRoundTrips(t *testing.T) {
	secrets := &memorySecrets{}
	store := NewCredentialStore(secrets)
	want := Credential{ClientID: "client", ClientSecret: "secret", AccessToken: "access", RefreshToken: "refresh"}
	if err := store.Save(chat.PlatformTwitch, "main", want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if secrets.service != KeyringService || secrets.user != "twitch/main" {
		t.Fatalf("keyring location = %q/%q", secrets.service, secrets.user)
	}
	got, err := store.Load(chat.PlatformTwitch, "main")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestCredentialStoreRejectsIncompleteCredentials(t *testing.T) {
	store := NewCredentialStore(&memorySecrets{})
	if err := store.Save(chat.PlatformYouTube, "main", Credential{ClientID: "client"}); err == nil {
		t.Fatal("incomplete credentials were accepted")
	}
}
