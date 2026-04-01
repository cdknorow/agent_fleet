package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// fetchProfileInfo retrieves the user's email and display name from the provider.
// Used for both initial connection setup and test connection.
func fetchProfileInfo(ctx context.Context, providerID, accessToken string) (email, name string, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	switch providerID {
	case "gmail", "google-calendar":
		return fetchGoogleProfile(ctx, accessToken)
	case "github":
		return fetchGitHubProfile(ctx, accessToken)
	case "slack":
		return fetchSlackProfile(ctx, accessToken)
	default:
		return "", "", fmt.Errorf("profile fetch not supported for provider: %s", providerID)
	}
}

func fetchGoogleProfile(ctx context.Context, token string) (string, string, error) {
	body, err := authGet(ctx, "https://www.googleapis.com/oauth2/v2/userinfo", token)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("parse google profile: %w", err)
	}
	return resp.Email, resp.Name, nil
}

func fetchGitHubProfile(ctx context.Context, token string) (string, string, error) {
	body, err := authGet(ctx, "https://api.github.com/user", token)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Login string `json:"login"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("parse github profile: %w", err)
	}
	name := resp.Name
	if name == "" {
		name = resp.Login
	}
	return resp.Email, name, nil
}

func fetchSlackProfile(ctx context.Context, token string) (string, string, error) {
	body, err := authGet(ctx, "https://slack.com/api/auth.test", token)
	if err != nil {
		return "", "", err
	}
	var resp struct {
		OK   bool   `json:"ok"`
		User string `json:"user"`
		Team string `json:"team"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", "", fmt.Errorf("parse slack profile: %w", err)
	}
	if !resp.OK {
		return "", "", fmt.Errorf("slack auth.test failed")
	}
	return "", resp.User + " (" + resp.Team + ")", nil
}

// revokeToken attempts to revoke an OAuth token at the provider (best-effort).
func revokeToken(providerID, token, clientID, clientSecret string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	switch providerID {
	case "gmail", "google-calendar":
		// Google revocation endpoint
		req, err := http.NewRequestWithContext(ctx, "POST",
			"https://oauth2.googleapis.com/revoke?token="+token, nil)
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		http.DefaultClient.Do(req)

	case "github":
		// GitHub token revocation
		if clientID == "" {
			return
		}
		req, err := http.NewRequestWithContext(ctx, "DELETE",
			"https://api.github.com/applications/"+clientID+"/token", nil)
		if err != nil {
			return
		}
		req.SetBasicAuth(clientID, clientSecret)
		http.DefaultClient.Do(req)

	case "slack":
		// Slack auth.revoke
		req, err := http.NewRequestWithContext(ctx, "GET",
			"https://slack.com/api/auth.revoke", nil)
		if err != nil {
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)
		http.DefaultClient.Do(req)
	}
}

// authGet performs a GET request with a Bearer token.
func authGet(ctx context.Context, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
