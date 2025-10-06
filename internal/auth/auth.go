package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"

	"golang.org/x/oauth2/google"
)

// Credentials holds authentication information
type Credentials struct {
	AccessToken string
	CredsFile   string // Path to temporary credentials file
}

// GetImpersonatedCredentials fetches an impersonated access token and creates a credentials file
func GetImpersonatedCredentials(ctx context.Context, serviceAccountEmail string) (*Credentials, error) {
	// Read application default credentials
	currentADC, err := applicationDefaultCredentials()
	if err != nil {
		return nil, err
	}

	// Fetch impersonated access token
	accessToken, err := fetchImpersonatedAccessToken(ctx, serviceAccountEmail)
	if err != nil {
		return nil, fmt.Errorf("fetch impersonated access token: %w", err)
	}

	// Create temporary credentials file for delegated impersonation
	credsFile, err := createDelegatedCredsFile(currentADC, serviceAccountEmail)
	if err != nil {
		return nil, fmt.Errorf("create credentials file: %w", err)
	}

	return &Credentials{
		AccessToken: accessToken,
		CredsFile:   credsFile,
	}, nil
}

// Cleanup removes the temporary credentials file
func (c *Credentials) Cleanup() error {
	if c.CredsFile == "" {
		return nil
	}
	return os.Remove(c.CredsFile)
}

// getGcloudConfigDir returns the gcloud configuration directory
func getGcloudConfigDir() (string, error) {
	// gcloud stores credentials in ~/.config/gcloud/ on all platforms
	// Respect CLOUDSDK_CONFIG if set, otherwise use ~/.config/gcloud
	configDir := os.Getenv("CLOUDSDK_CONFIG")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get user home directory: %w", err)
		}
		configDir = filepath.Join(home, ".config", "gcloud")
	}
	return configDir, nil
}

// applicationDefaultCredentials reads the local application default credentials
func applicationDefaultCredentials() (string, error) {
	configDir, err := getGcloudConfigDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(configDir, "application_default_credentials.json")

	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("no application default credentials found. Please authenticate using 'gcloud auth application-default login'")
		}
		return "", fmt.Errorf("read application default credentials at %s: %w", path, err)
	}

	return string(b), nil
}

// fetchImpersonatedAccessToken generates an access token for the service account
func fetchImpersonatedAccessToken(ctx context.Context, serviceAccountEmail string) (string, error) {
	// Get credentials from application default credentials
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("find default credentials: %w", err)
	}

	// Get access token
	token, err := creds.TokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("get access token: %w", err)
	}

	accessToken := token.AccessToken
	if accessToken == "" {
		return "", fmt.Errorf("got empty access token")
	}

	// Generate access token for delegated service account
	body := struct {
		Delegates []string `json:"delegates"`
		Scope     []string `json:"scope"`
	}{
		Delegates: []string{"projects/-/serviceAccounts/" + serviceAccountEmail},
		Scope:     []string{"https://www.googleapis.com/auth/cloud-platform"},
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request body: %w", err)
	}

	url := fmt.Sprintf(
		"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken",
		serviceAccountEmail,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to generate access token (status %d): %s", resp.StatusCode, string(b))
	}

	var tokens struct {
		AccessToken string `json:"accessToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokens); err != nil {
		return "", err
	}

	return tokens.AccessToken, nil
}

// createDelegatedCredsFile creates a temporary credentials file with impersonation config
func createDelegatedCredsFile(currentADC, serviceAccountEmail string) (string, error) {
	serviceAccountImpersonationURL := fmt.Sprintf(
		"https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken",
		serviceAccountEmail,
	)

	delegateCreds := struct {
		Delegates                      []string        `json:"delegates"`
		Type                           string          `json:"type"`
		ServiceAccountImpersonationURL string          `json:"service_account_impersonation_url"`
		SourceCredentials              json.RawMessage `json:"source_credentials"`
	}{
		Delegates:                      []string{"projects/-/serviceAccounts/" + serviceAccountEmail},
		Type:                           "impersonated_service_account",
		ServiceAccountImpersonationURL: serviceAccountImpersonationURL,
		SourceCredentials:              json.RawMessage(currentADC),
	}

	delegateCredsJSON, err := json.Marshal(delegateCreds)
	if err != nil {
		return "", err
	}

	// Create temp directory
	tempDir := os.TempDir()
	credsPath := filepath.Join(tempDir, fmt.Sprintf("cloudrun-local-creds-%s.json", randomLower(8)))

	if err := os.WriteFile(credsPath, delegateCredsJSON, 0o600); err != nil {
		return "", err
	}

	return credsPath, nil
}

// randomLower generates a random lowercase string of length n
func randomLower(n int) string {
	b := make([]rune, n)
	for i := range b {
		//nolint:gosec // we don't need a secure randomizer for this
		b[i] = rune(rand.IntN(26) + 97) // 'a' is 97, 'z' is 122
	}
	return string(b)
}
