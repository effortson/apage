package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apage/apage/internal/audit"
	"github.com/apage/apage/internal/httpx"
	"github.com/apage/apage/internal/id"
	"github.com/apage/apage/internal/store"
	"github.com/go-chi/chi/v5"
)

const oauthStateCookie = "apage_oauth_state"

var oauthHTTP = &http.Client{Timeout: 15 * time.Second}

// oauthProvider describes one OAuth2 identity provider (spec §25).
type oauthProvider struct {
	name       string
	authURL    string
	tokenURL   string
	scope      string
	clientID   string
	secret     string
	fetchEmail func(accessToken string) (email string, verified bool, err error)
}

// provider resolves a configured provider by name; ok is false when the provider
// has no client id/secret (OAuth disabled for it).
func (s *Server) provider(name string) (*oauthProvider, bool) {
	switch name {
	case "github":
		if s.cfg.OAuthGitHubClientID == "" || s.cfg.OAuthGitHubSecret == "" {
			return nil, false
		}
		return &oauthProvider{
			name: "github", authURL: "https://github.com/login/oauth/authorize",
			tokenURL: "https://github.com/login/oauth/access_token", scope: "read:user user:email",
			clientID: s.cfg.OAuthGitHubClientID, secret: s.cfg.OAuthGitHubSecret, fetchEmail: githubEmail,
		}, true
	case "google":
		if s.cfg.OAuthGoogleClientID == "" || s.cfg.OAuthGoogleSecret == "" {
			return nil, false
		}
		return &oauthProvider{
			name: "google", authURL: "https://accounts.google.com/o/oauth2/v2/auth",
			tokenURL: "https://oauth2.googleapis.com/token", scope: "openid email",
			clientID: s.cfg.OAuthGoogleClientID, secret: s.cfg.OAuthGoogleSecret, fetchEmail: googleEmail,
		}, true
	}
	return nil, false
}

func (s *Server) redirectURI(provider string) string {
	return s.cfg.OAuthRedirectBase + "/api/v1/auth/oauth/" + provider + "/callback"
}

// handleOAuthProviders lists the OAuth providers that are configured, so the
// login/register UI can render the matching buttons (spec §25).
func (s *Server) handleOAuthProviders(w http.ResponseWriter, r *http.Request) {
	providers := []string{}
	for _, name := range []string{"github", "google"} {
		if _, ok := s.provider(name); ok {
			providers = append(providers, name)
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"providers": providers})
}

// handleOAuthStart redirects the visitor to the provider's consent screen with a
// CSRF state value bound to a cookie (spec §25).
func (s *Server) handleOAuthStart(w http.ResponseWriter, r *http.Request) {
	p, ok := s.provider(chi.URLParam(r, "provider"))
	if !ok {
		httpx.NotFound(w, r) // provider not configured
		return
	}
	state := id.NewSecret("oas_")
	http.SetCookie(w, &http.Cookie{
		Name: oauthStateCookie, Value: state, Path: "/api/v1/auth/oauth",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 600,
	})
	q := url.Values{}
	q.Set("client_id", p.clientID)
	q.Set("redirect_uri", s.redirectURI(p.name))
	q.Set("scope", p.scope)
	q.Set("state", state)
	q.Set("response_type", "code")
	http.Redirect(w, r, p.authURL+"?"+q.Encode(), http.StatusFound)
}

// handleOAuthCallback verifies state, exchanges the code, resolves a verified
// email, then logs in (or provisions) the matching account (spec §25).
func (s *Server) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	p, ok := s.provider(chi.URLParam(r, "provider"))
	if !ok {
		httpx.NotFound(w, r)
		return
	}
	// CSRF state must match the cookie set at /start.
	c, err := r.Cookie(oauthStateCookie)
	if err != nil || c.Value == "" || r.URL.Query().Get("state") != c.Value {
		httpx.Forbidden(w, r, "invalid oauth state")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/api/v1/auth/oauth", MaxAge: -1})
	code := r.URL.Query().Get("code")
	if code == "" {
		httpx.BadRequest(w, r, "missing authorization code")
		return
	}
	token, err := s.exchangeCode(r.Context(), p, code)
	if err != nil {
		s.log.Warn("oauth token exchange", "provider", p.name, "err", err)
		httpx.Err(w, r, http.StatusBadGateway, httpx.CodeInternal, "oauth exchange failed", true)
		return
	}
	email, verified, err := p.fetchEmail(token)
	if err != nil || email == "" {
		httpx.Err(w, r, http.StatusBadGateway, httpx.CodeInternal, "could not read account email", true)
		return
	}
	if !verified {
		httpx.Forbidden(w, r, "provider email is not verified")
		return
	}
	email = strings.ToLower(strings.TrimSpace(email))

	userID, err := s.loginOrProvision(r, email)
	if err != nil {
		s.log.Error("oauth provision", "err", err)
		httpx.Internal(w, r)
		return
	}
	s.startSession(w, r, userID)
	// Land the browser on the console (the callback runs on the console origin).
	http.Redirect(w, r, "/console", http.StatusFound)
}

// loginOrProvision links by verified email: an existing user logs in; otherwise a
// new oauth-backed account (User + Tenant + owner Membership + Quota) is created.
func (s *Server) loginOrProvision(r *http.Request, email string) (string, error) {
	if u, err := s.db.UserByEmail(r.Context(), email); err == nil {
		return u.UserID, nil
	}
	tenant := store.Tenant{TenantID: id.New(id.PrefixTenant), Name: email, Plan: "lite", TrustLevel: "new"}
	user := store.User{UserID: id.New(id.PrefixUser), Email: email, AuthProvider: "oauth"}
	if err := s.db.RegisterAccount(r.Context(), tenant, user, id.New(id.PrefixMembership)); err != nil {
		return "", err
	}
	_ = s.db.MarkEmailVerified(r.Context(), user.UserID) // provider already verified it
	s.audit(r.Context(), audit.Entry{TenantID: tenant.TenantID, Event: audit.UserRegistered,
		ActorType: audit.ActorUser, ActorID: user.UserID, IP: httpx.ClientIP(r.Context()), Reason: "oauth"})
	return user.UserID, nil
}

// exchangeCode swaps an authorization code for an access token.
func (s *Server) exchangeCode(ctx context.Context, p *oauthProvider, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", p.clientID)
	form.Set("client_secret", p.secret)
	form.Set("code", code)
	form.Set("redirect_uri", s.redirectURI(p.name))
	form.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.AccessToken == "" {
		return "", errOAuthNoToken
	}
	return body.AccessToken, nil
}

var errOAuthNoToken = &oauthErr{"no access_token in provider response"}

type oauthErr struct{ s string }

func (e *oauthErr) Error() string { return e.s }

// githubEmail returns the primary verified email from the GitHub API.
func githubEmail(token string) (string, bool, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/user/emails", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", false, err
	}
	for _, e := range emails {
		if e.Primary {
			return e.Email, e.Verified, nil
		}
	}
	if len(emails) > 0 {
		return emails[0].Email, emails[0].Verified, nil
	}
	return "", false, nil
}

// googleEmail returns the email + verification flag from Google's userinfo.
func googleEmail(token string) (string, bool, error) {
	req, _ := http.NewRequest(http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	var info struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", false, err
	}
	return info.Email, info.EmailVerified, nil
}
