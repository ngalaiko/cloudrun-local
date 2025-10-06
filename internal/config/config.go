package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2/google"
	"gopkg.in/yaml.v3"
)

// Config represents a parsed Cloud Run service configuration
type Config struct {
	ServiceName     string
	ServiceAccount  string
	ProjectID       string
	EnvironmentVars []EnvVar
}

// EnvVar represents an environment variable from the config
type EnvVar struct {
	Name      string
	Value     string
	SecretRef *SecretRef
}

// SecretRef represents a reference to a secret in Secret Manager
type SecretRef struct {
	Name string
	Key  string
}

// Parse reads and parses a Cloud Run YAML configuration file (Service or Job)
func Parse(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Unmarshal YAML into a generic map
	var yamlRaw map[string]any
	if err := yaml.Unmarshal(data, &yamlRaw); err != nil {
		return nil, fmt.Errorf("unmarshal yaml: %w", err)
	}

	// Convert to JSON for easier typed parsing
	jsonData, err := json.Marshal(yamlRaw)
	if err != nil {
		return nil, fmt.Errorf("marshal to json: %w", err)
	}

	// Check the kind to determine if it's a Service or Job
	var kindCheck struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(jsonData, &kindCheck); err != nil {
		return nil, fmt.Errorf("unmarshal kind: %w", err)
	}

	switch kindCheck.Kind {
	case "Service":
		return parseService(jsonData)
	case "Job":
		return parseJob(jsonData)
	default:
		return nil, fmt.Errorf("unsupported kind: %s (expected Service or Job)", kindCheck.Kind)
	}
}

// parseService parses a Cloud Run Service configuration
func parseService(jsonData []byte) (*Config, error) {
	var raw struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Spec struct {
					ServiceAccountName string `json:"serviceAccountName"`
					Containers         []struct {
						Env []struct {
							Name      string `json:"name"`
							Value     string `json:"value"`
							ValueFrom struct {
								SecretKeyRef struct {
									Name string `json:"name"`
									Key  string `json:"key"`
								} `json:"secretKeyRef"`
							} `json:"valueFrom"`
						} `json:"env"`
					} `json:"containers"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal service json: %w", err)
	}

	if len(raw.Spec.Template.Spec.Containers) != 1 {
		return nil, fmt.Errorf("expected exactly 1 container, got %d", len(raw.Spec.Template.Spec.Containers))
	}

	serviceAccount := raw.Spec.Template.Spec.ServiceAccountName
	if serviceAccount == "" {
		return nil, fmt.Errorf("serviceAccountName not found in config")
	}

	projectID, err := extractProjectID(serviceAccount)
	if err != nil {
		return nil, fmt.Errorf("extract project ID: %w", err)
	}

	envVars := parseEnvVars(raw.Spec.Template.Spec.Containers[0].Env)

	return &Config{
		ServiceName:     raw.Metadata.Name,
		ServiceAccount:  serviceAccount,
		ProjectID:       projectID,
		EnvironmentVars: envVars,
	}, nil
}

// parseJob parses a Cloud Run Job configuration
func parseJob(jsonData []byte) (*Config, error) {
	var raw struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
		Spec struct {
			Template struct {
				Spec struct {
					Template struct {
						Spec struct {
							ServiceAccountName string `json:"serviceAccountName"`
							Containers         []struct {
								Env []struct {
									Name      string `json:"name"`
									Value     string `json:"value"`
									ValueFrom struct {
										SecretKeyRef struct {
											Name string `json:"name"`
											Key  string `json:"key"`
										} `json:"secretKeyRef"`
									} `json:"valueFrom"`
								} `json:"env"`
							} `json:"containers"`
						} `json:"spec"`
					} `json:"template"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}

	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal job json: %w", err)
	}

	if len(raw.Spec.Template.Spec.Template.Spec.Containers) != 1 {
		return nil, fmt.Errorf("expected exactly 1 container, got %d", len(raw.Spec.Template.Spec.Template.Spec.Containers))
	}

	serviceAccount := raw.Spec.Template.Spec.Template.Spec.ServiceAccountName
	if serviceAccount == "" {
		return nil, fmt.Errorf("serviceAccountName not found in config")
	}

	projectID, err := extractProjectID(serviceAccount)
	if err != nil {
		return nil, fmt.Errorf("extract project ID: %w", err)
	}

	envVars := parseEnvVars(raw.Spec.Template.Spec.Template.Spec.Containers[0].Env)

	return &Config{
		ServiceName:     raw.Metadata.Name,
		ServiceAccount:  serviceAccount,
		ProjectID:       projectID,
		EnvironmentVars: envVars,
	}, nil
}

// parseEnvVars parses environment variables from container env array
func parseEnvVars(envArray []struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	ValueFrom struct {
		SecretKeyRef struct {
			Name string `json:"name"`
			Key  string `json:"key"`
		} `json:"secretKeyRef"`
	} `json:"valueFrom"`
}) []EnvVar {
	var envVars []EnvVar
	for _, env := range envArray {
		envVar := EnvVar{Name: env.Name}

		if env.Value != "" {
			envVar.Value = env.Value
		} else if env.ValueFrom.SecretKeyRef.Name != "" && env.ValueFrom.SecretKeyRef.Key != "" {
			envVar.SecretRef = &SecretRef{
				Name: env.ValueFrom.SecretKeyRef.Name,
				Key:  env.ValueFrom.SecretKeyRef.Key,
			}
		}

		envVars = append(envVars, envVar)
	}
	return envVars
}

// extractProjectID extracts the project ID from a service account email
// Expected format: name@project-id.iam.gserviceaccount.com
func extractProjectID(serviceAccount string) (string, error) {
	parts := strings.Split(serviceAccount, "@")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid service account format: %s", serviceAccount)
	}

	domainParts := strings.Split(parts[1], ".")
	if len(domainParts) < 1 {
		return "", fmt.Errorf("invalid service account domain: %s", parts[1])
	}

	projectID := domainParts[0]
	if projectID == "" {
		return "", fmt.Errorf("could not extract project ID from service account: %s", serviceAccount)
	}

	return projectID, nil
}

// GetDefaultProjectID returns the default project ID from application default credentials
func GetDefaultProjectID(ctx context.Context) (string, error) {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return "", fmt.Errorf("find default credentials: %w", err)
	}

	projectID := creds.ProjectID
	if projectID == "" {
		return "", fmt.Errorf("no project ID found in default credentials")
	}

	return projectID, nil
}
