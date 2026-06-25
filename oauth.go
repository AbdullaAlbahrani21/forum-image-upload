package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"forum/database"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ── Config ────────────────────────────────────────────────────────────────────

var (
	googleClientID     = getEnv("GOOGLE_CLIENT_ID", "")
	googleClientSecret = getEnv("GOOGLE_CLIENT_SECRET", "")
	googleRedirectURI  = getEnv("GOOGLE_REDIRECT_URI", "http://127.0.0.1:8080/auth/google/callback")

	githubClientID     = getEnv("GITHUB_CLIENT_ID", "")
	githubClientSecret = getEnv("GITHUB_CLIENT_SECRET", "")
	githubRedirectURI  = getEnv("GITHUB_REDIRECT_URI", "http://127.0.0.1:8080/auth/github/callback")
)

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

var (
	oauthStates   = make(map[string]time.Time)
	oauthStatesMu sync.Mutex
)

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	state := base64.URLEncoding.EncodeToString(b)

	oauthStatesMu.Lock()
	oauthStates[state] = time.Now().Add(10 * time.Minute)
	for k, exp := range oauthStates {
		if time.Now().After(exp) {
			delete(oauthStates, k)
		}
	}
	oauthStatesMu.Unlock()

	return state, nil
}

func validateState(state string) bool {
	if state == "" {
		log.Println("OAuth: empty state parameter")
		return false
	}
	oauthStatesMu.Lock()
	defer oauthStatesMu.Unlock()
	exp, ok := oauthStates[state]
	if !ok {
		log.Println("OAuth: state not found in store")
		return false
	}
	delete(oauthStates, state)
	if time.Now().After(exp) {
		log.Println("OAuth: state expired")
		return false
	}
	return true
}

// ── Google OAuth ──────────────────────────────────────────────────────────────

func googleLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		RenderError(w, r, 405)
		return
	}

	if _, ok := getSession(r); ok {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	state, err := generateState()
	if err != nil {
		log.Println("OAuth state generation error:", err)
		RenderError(w, r, 500)
		return
	}

	params := url.Values{}
	params.Set("client_id", googleClientID)
	params.Set("redirect_uri", googleRedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "openid email profile")
	params.Set("state", state)
	params.Set("access_type", "online")

	http.Redirect(w, r,
		"https://accounts.google.com/o/oauth2/v2/auth?"+params.Encode(),
		http.StatusTemporaryRedirect)
}

func googleCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		RenderError(w, r, 405)
		return
	}

	state := r.URL.Query().Get("state")

	if r.URL.Query().Get("error") != "" {
		if state != "" {
			validateState(state)
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Validate state
	if !validateState(state) {
		RenderError(w, r, 400)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		RenderError(w, r, 400)
		return
	}

	token, err := exchangeGoogleCode(code)
	if err != nil {
		log.Println("Google token exchange error:", err)
		RenderError(w, r, 500)
		return
	}

	info, err := getGoogleUserInfo(token)
	if err != nil {
		log.Println("Google user info error:", err)
		RenderError(w, r, 500)
		return
	}

	handleOAuthUser(w, r, "google", info.Sub, info.Email, info.Name)
}

func exchangeGoogleCode(code string) (string, error) {
	data := url.Values{}
	data.Set("code", code)
	data.Set("client_id", googleClientID)
	data.Set("client_secret", googleClientSecret)
	data.Set("redirect_uri", googleRedirectURI)
	data.Set("grant_type", "authorization_code")

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("google token error: %s", result.Error)
	}
	return result.AccessToken, nil
}

type googleUserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

func getGoogleUserInfo(accessToken string) (*googleUserInfo, error) {
	req, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info googleUserInfo
	if err = json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	if info.Sub == "" {
		return nil, fmt.Errorf("empty Google user ID")
	}
	return &info, nil
}

// ── GitHub OAuth ──────────────────────────────────────────────────────────────

func githubLoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		RenderError(w, r, 405)
		return
	}

	if _, ok := getSession(r); ok {
		http.Redirect(w, r, "/home", http.StatusSeeOther)
		return
	}

	state, err := generateState()
	if err != nil {
		log.Println("OAuth state generation error:", err)
		RenderError(w, r, 500)
		return
	}

	params := url.Values{}
	params.Set("client_id", githubClientID)
	params.Set("redirect_uri", githubRedirectURI)
	params.Set("scope", "user:email")
	params.Set("state", state)

	http.Redirect(w, r,
		"https://github.com/login/oauth/authorize?"+params.Encode(),
		http.StatusTemporaryRedirect)
}

func githubCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		RenderError(w, r, 405)
		return
	}

	state := r.URL.Query().Get("state")

	if r.URL.Query().Get("error") != "" {
		if state != "" {
			validateState(state)
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if !validateState(state) {
		RenderError(w, r, 400)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		RenderError(w, r, 400)
		return
	}

	token, err := exchangeGithubCode(code)
	if err != nil {
		log.Println("GitHub token exchange error:", err)
		RenderError(w, r, 500)
		return
	}

	info, err := getGithubUserInfo(token)
	if err != nil {
		log.Println("GitHub user info error:", err)
		RenderError(w, r, 500)
		return
	}

	handleOAuthUser(w, r, "github", info.ID, info.Email, info.Login)
}

func exchangeGithubCode(code string) (string, error) {
	data := url.Values{}
	data.Set("client_id", githubClientID)
	data.Set("client_secret", githubClientSecret)
	data.Set("code", code)
	data.Set("redirect_uri", githubRedirectURI)

	req, err := http.NewRequest("POST",
		"https://github.com/login/oauth/access_token",
		strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err = json.Unmarshal(body, &result); err != nil {
		return "", err
	}
	if result.Error != "" {
		return "", fmt.Errorf("github token error: %s", result.Error)
	}
	return result.AccessToken, nil
}

type githubUserInfo struct {
	ID    string `json:"-"`
	IDInt int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email"`
}

func getGithubUserInfo(accessToken string) (*githubUserInfo, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info githubUserInfo
	if err = json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	info.ID = fmt.Sprintf("%d", info.IDInt)

	if info.Email == "" {
		info.Email, _ = getGithubPrimaryEmail(accessToken)
	}

	if info.IDInt == 0 {
		return nil, fmt.Errorf("empty GitHub user ID")
	}
	return &info, nil
}

func getGithubPrimaryEmail(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err = json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	if len(emails) > 0 {
		return emails[0].Email, nil
	}
	return "", fmt.Errorf("no email found")
}

// ── Shared OAuth user handling ────────────────────────────────────────────────

func handleOAuthUser(w http.ResponseWriter, r *http.Request, provider, providerID, email, displayName string) {

	userID, err := database.GetUserByProviderID(provider, providerID)
	if err == nil {
		startSessionAndRedirect(w, r, userID)
		return
	}

	if email != "" {
		userID, err = database.GetUserIDByEmail(email)
		if err == nil {
			if linkErr := database.LinkOAuthProvider(userID, provider, providerID); linkErr != nil {
				log.Println("Error linking OAuth provider:", linkErr)
			}
			startSessionAndRedirect(w, r, userID)
			return
		}
	}

	username := sanitizeOAuthUsername(displayName)
	username = database.EnsureUniqueUsername(username)

	if email == "" {
		email = fmt.Sprintf("%s_%s@%s.oauth", provider, providerID, provider)
	}

	userID, err = database.CreateOAuthUser(username, email, provider, providerID)
	if err != nil {
		log.Println("Error creating OAuth user:", err)
		RenderError(w, r, 500)
		return
	}

	startSessionAndRedirect(w, r, userID)
}

func startSessionAndRedirect(w http.ResponseWriter, r *http.Request, userID int) {
	database.DeleteSessionsByUserID(userID)

	sessionID, err := database.CreateSession(userID)
	if err != nil {
		log.Println("Error creating session:", err)
		RenderError(w, r, 500)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    sessionID,
		Path:     "/",
		MaxAge:   3600 * 24,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func sanitizeOAuthUsername(name string) string {
	var b strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			b.WriteRune(ch)
		}
	}
	result := b.String()
	if len(result) < 3 {
		result = result + "user"
	}
	if len(result) > 30 {
		result = result[:30]
	}
	return strings.ToLower(result)
}
