package env

import (
	"context"
	"fmt"

	"github.com/ngalaiko/cloudrun-local/internal/auth"
	"github.com/ngalaiko/cloudrun-local/internal/config"
	"github.com/ngalaiko/cloudrun-local/internal/secrets"
)

// Resolver resolves environment variables from a Cloud Run config
type Resolver struct {
	config *config.Config
	creds  *auth.Credentials
}

// NewResolver creates a new environment resolver
func NewResolver(ctx context.Context, cfg *config.Config) (*Resolver, error) {
	creds, err := auth.GetImpersonatedCredentials(ctx, cfg.ServiceAccount)
	if err != nil {
		return nil, fmt.Errorf("get impersonated credentials: %w", err)
	}

	return &Resolver{
		config: cfg,
		creds:  creds,
	}, nil
}

// Resolve returns all environment variables as KEY=value strings
func (r *Resolver) Resolve(ctx context.Context) ([]string, error) {
	result := make([]string, 0, len(r.config.EnvironmentVars)+10)

	// Add Cloud Run metadata environment variables
	if r.config.ServiceName != "" {
		result = append(result, "K_SERVICE="+r.config.ServiceName)
	}
	result = append(result, "K_REVISION=local")
	result = append(result, "GOOGLE_CLOUD_PROJECT="+r.config.ProjectID)
	result = append(result, "GOOGLE_APPLICATION_CREDENTIALS="+r.creds.CredsFile)

	// Resolve user-defined environment variables
	secretsClient := secrets.NewClient(r.creds.AccessToken, r.config.ProjectID)

	for _, envVar := range r.config.EnvironmentVars {
		if envVar.Value != "" {
			// Simple value
			result = append(result, envVar.Name+"="+envVar.Value)
			continue
		}

		if envVar.SecretRef != nil {
			// Secret reference - fetch from Secret Manager
			secretValue, err := secretsClient.AccessSecretVersion(
				ctx,
				envVar.SecretRef.Name,
				envVar.SecretRef.Key,
			)
			if err != nil {
				return nil, fmt.Errorf("access secret %s: %w", envVar.SecretRef.Name, err)
			}
			result = append(result, envVar.Name+"="+secretValue)
		}
	}

	return result, nil
}

// Cleanup removes temporary files created during resolution
func (r *Resolver) Cleanup() error {
	if r.creds != nil {
		return r.creds.Cleanup()
	}
	return nil
}
