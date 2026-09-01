package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wingitman/streamy/internal/chat"
)

func TestOAuthAuthorizationURLUsesStateAndPKCE(t *testing.T) {
	flow := NewOAuthFlow(chat.PlatformTwitch, OAuthApplication{
		ClientID:    "client-id",
		RedirectURL: OAuthRedirectURL,
		Scopes:      []string{"user:read:chat"},
	}, "secret")
	flow.Endpoints.AuthorizationURL = "https://example.test/authorize"
	values, err := url.ParseQuery(strings.Split(flow.authorizationURL("state", "challenge"), "?")[1])
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	if values.Get("state") != "state" || values.Get("code_challenge") != "challenge" || values.Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization parameters = %#v", values)
	}
}

func TestOAuthExchangeReturnsCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		if !strings.Contains(string(body), "code_verifier=verifier") {
			t.Errorf("request body does not contain verifier: %s", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"access_token":"access","refresh_token":"refresh"}`)
	}))
	defer server.Close()
	flow := NewOAuthFlow(chat.PlatformYouTube, OAuthApplication{ClientID: "client", RedirectURL: OAuthRedirectURL}, "secret")
	flow.Client = server.Client()
	flow.Endpoints.TokenURL = server.URL
	credential, err := flow.exchange(context.Background(), "code", "verifier")
	if err != nil {
		t.Fatalf("exchange() error = %v", err)
	}
	if credential.AccessToken != "access" || credential.RefreshToken != "refresh" || credential.ClientID != "client" {
		t.Fatalf("credential = %#v", credential)
	}
}

func TestRefreshPreservesRefreshTokenWhenProviderOmitsIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.FormValue("grant_type") != "refresh_token" {
			t.Errorf("refresh request = %s %s", request.Method, request.URL)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600}`)
	}))
	defer server.Close()
	flow := &OAuthFlow{
		Client: http.DefaultClient, ClientID: "client", ClientSecret: "secret",
		CallbackURL: OAuthRedirectURL, Endpoints: OAuthEndpoints{TokenURL: server.URL},
	}
	credential, err := flow.Refresh(context.Background(), Credential{RefreshToken: "refresh"})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if credential.AccessToken != "new-access" || credential.RefreshToken != "refresh" {
		t.Fatalf("refreshed credential = %#v", credential)
	}
}
