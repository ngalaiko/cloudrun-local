package secrets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

// Client handles Secret Manager API access
type Client struct {
	accessToken string
	projectID   string
}

// NewClient creates a new Secret Manager client
func NewClient(accessToken, projectID string) *Client {
	return &Client{
		accessToken: accessToken,
		projectID:   projectID,
	}
}

// AccessSecretVersion retrieves a secret value from Secret Manager
func (c *Client) AccessSecretVersion(ctx context.Context, secretName, version string) (string, error) {
	secretPath := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", c.projectID, secretName, version)

	url := fmt.Sprintf("https://secretmanager.googleapis.com/v1/%s:access", secretPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("expected 200 response status, received %d", resp.StatusCode)
	}

	var responseBody struct {
		Payload struct {
			Data string `json:"data"`
		} `json:"payload"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&responseBody); err != nil {
		return "", err
	}

	if responseBody.Payload.Data == "" {
		return "", fmt.Errorf("no value for secret %s", secretPath)
	}

	decodedData, err := base64.StdEncoding.DecodeString(responseBody.Payload.Data)
	if err != nil {
		return "", err
	}

	return string(decodedData), nil
}
