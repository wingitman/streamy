package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/wingitman/streamy/internal/chat"
	"golang.org/x/oauth2"
)

type OAuthEndpoints struct {
	AuthorizationURL string
	TokenURL         string
}

func Endpoints(platform chat.Platform) OAuthEndpoints {
	switch platform {
	case chat.PlatformTwitch:
		return OAuthEndpoints{
			AuthorizationURL: TwitchAuthorization,
			TokenURL:         "https://id.twitch.tv/oauth2/token",
		}
	case chat.PlatformYouTube:
		return OAuthEndpoints{
			AuthorizationURL: YouTubeAuthorization,
			TokenURL:         "https://oauth2.googleapis.com/token",
		}
	default:
		return OAuthEndpoints{}
	}
}

type OAuthFlow struct {
	Client       *http.Client
	Listen       func(network, address string) (net.Listener, error)
	OpenBrowser  func(string) error
	Now          func() time.Time
	Random       io.Reader
	CallbackURL  string
	Endpoints    OAuthEndpoints
	ClientID     string
	ClientSecret string
	Scopes       []string
}

func NewOAuthFlow(platform chat.Platform, application OAuthApplication, clientSecret string) *OAuthFlow {
	return &OAuthFlow{
		Client:       http.DefaultClient,
		Listen:       net.Listen,
		OpenBrowser:  openBrowser,
		Now:          time.Now,
		Random:       rand.Reader,
		CallbackURL:  application.RedirectURL,
		Endpoints:    Endpoints(platform),
		ClientID:     application.ClientID,
		ClientSecret: clientSecret,
		Scopes:       application.Scopes,
	}
}

func (f *OAuthFlow) Authorize(ctx context.Context) (Credential, error) {
	if f.CallbackURL != OAuthRedirectURL || f.ClientID == "" || f.ClientSecret == "" {
		return Credential{}, errors.New("OAuth flow has invalid callback or client credentials")
	}
	if f.Endpoints.AuthorizationURL == "" || f.Endpoints.TokenURL == "" {
		return Credential{}, errors.New("OAuth endpoints are not configured")
	}
	state, err := randomString(f.Random, 32)
	if err != nil {
		return Credential{}, fmt.Errorf("generate OAuth state: %w", err)
	}
	verifier, err := randomString(f.Random, 32)
	if err != nil {
		return Credential{}, fmt.Errorf("generate PKCE verifier: %w", err)
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	listener, err := f.Listen("tcp", "127.0.0.1:43821")
	if err != nil {
		return Credential{}, fmt.Errorf("listen for OAuth callback: %w", err)
	}
	defer listener.Close()

	code := make(chan string, 1)
	callbackErr := make(chan error, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("state") != state {
			callbackErr <- errors.New("OAuth state mismatch")
			http.Error(writer, "Authorization state mismatch", http.StatusBadRequest)
			return
		}
		if callbackError := query.Get("error"); callbackError != "" {
			callbackErr <- fmt.Errorf("OAuth authorization failed: %s", callbackError)
			_, _ = io.WriteString(writer, "Authorization failed. You can close this window.")
			return
		}
		if query.Get("code") == "" {
			callbackErr <- errors.New("OAuth callback did not contain a code")
			http.Error(writer, "Missing authorization code", http.StatusBadRequest)
			return
		}
		code <- query.Get("code")
		_, _ = io.WriteString(writer, "Authorization complete. You can close this window.")
	})}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			callbackErr <- err
		}
	}()

	authorizationURL := f.authorizationURL(state, challenge)
	if err := f.OpenBrowser(authorizationURL); err != nil {
		_ = server.Shutdown(context.Background())
		return Credential{}, fmt.Errorf("open OAuth browser: %w", err)
	}
	select {
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return Credential{}, ctx.Err()
	case err := <-callbackErr:
		_ = server.Shutdown(context.Background())
		return Credential{}, err
	case authorizationCode := <-code:
		_ = server.Shutdown(context.Background())
		return f.exchange(ctx, authorizationCode, verifier)
	}
}

// Refresh exchanges a stored refresh token without opening a browser. The
// returned credential keeps the old refresh token when the provider omits it.
func (f *OAuthFlow) Refresh(ctx context.Context, credential Credential) (Credential, error) {
	if f.Client == nil {
		f.Client = http.DefaultClient
	}
	if credential.RefreshToken == "" || f.ClientID == "" || f.ClientSecret == "" || f.Endpoints.TokenURL == "" {
		return Credential{}, errors.New("OAuth refresh has invalid client or refresh-token settings")
	}
	clientConfig := oauth2.Config{
		ClientID:     f.ClientID,
		ClientSecret: f.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  f.Endpoints.AuthorizationURL,
			TokenURL: f.Endpoints.TokenURL,
		},
		Scopes:      f.Scopes,
		RedirectURL: f.CallbackURL,
	}
	token := &oauth2.Token{AccessToken: credential.AccessToken, RefreshToken: credential.RefreshToken, Expiry: time.Unix(1, 0)}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, f.Client)
	refreshed, err := clientConfig.TokenSource(ctx, token).Token()
	if err != nil {
		return Credential{}, fmt.Errorf("refresh OAuth token: %w", err)
	}
	if refreshed.AccessToken == "" {
		return Credential{}, errors.New("OAuth refresh response has no access token")
	}
	refreshToken := refreshed.RefreshToken
	if refreshToken == "" {
		refreshToken = credential.RefreshToken
	}
	expiresAt := int64(0)
	if !refreshed.Expiry.IsZero() {
		expiresAt = refreshed.Expiry.Unix()
	}
	return Credential{ClientID: f.ClientID, ClientSecret: f.ClientSecret, AccessToken: refreshed.AccessToken, RefreshToken: refreshToken, ExpiresAt: expiresAt}, nil
}

func (f *OAuthFlow) authorizationURL(state, challenge string) string {
	query := url.Values{
		"client_id":             {f.ClientID},
		"redirect_uri":          {f.CallbackURL},
		"response_type":         {"code"},
		"scope":                 {strings.Join(f.Scopes, " ")},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	return f.Endpoints.AuthorizationURL + "?" + query.Encode()
}

func (f *OAuthFlow) exchange(ctx context.Context, code, verifier string) (Credential, error) {
	form := url.Values{
		"client_id":     {f.ClientID},
		"client_secret": {f.ClientSecret},
		"code":          {code},
		"code_verifier": {verifier},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {f.CallbackURL},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, f.Endpoints.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Credential{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := f.Client.Do(request)
	if err != nil {
		return Credential{}, fmt.Errorf("exchange OAuth code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return Credential{}, fmt.Errorf("exchange OAuth code: HTTP %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	var token struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(response.Body).Decode(&token); err != nil {
		return Credential{}, fmt.Errorf("decode OAuth token: %w", err)
	}
	if token.AccessToken == "" {
		return Credential{}, errors.New("OAuth token response has no access token")
	}
	expiresAt := int64(0)
	if token.ExpiresIn > 0 {
		now := time.Now()
		if f.Now != nil {
			now = f.Now()
		}
		expiresAt = now.Add(time.Duration(token.ExpiresIn) * time.Second).Unix()
	}
	return Credential{ClientID: f.ClientID, ClientSecret: f.ClientSecret, AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, ExpiresAt: expiresAt}, nil
}

func randomString(reader io.Reader, size int) (string, error) {
	value := make([]byte, size)
	if _, err := io.ReadFull(reader, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func openBrowser(target string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{target}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	default:
		command, args = "xdg-open", []string{target}
	}
	return exec.Command(command, args...).Start()
}
