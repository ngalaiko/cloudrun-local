package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/ngalaiko/cloudrun-local/internal/config"
	"github.com/ngalaiko/cloudrun-local/internal/env"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse flags
	var (
		configFile  string
		showVersion bool
		showHelp    bool
	)

	flag.StringVar(&configFile, "config", "service.yaml", "Path to Cloud Run service YAML config file")
	flag.StringVar(&configFile, "c", "service.yaml", "Path to Cloud Run service YAML config file (shorthand)")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.BoolVar(&showVersion, "v", false, "Show version information (shorthand)")
	flag.BoolVar(&showHelp, "help", false, "Show help information")
	flag.BoolVar(&showHelp, "h", false, "Show help information (shorthand)")

	flag.Parse()

	if showVersion {
		fmt.Printf("cloudrun-local version %s\n", version)
		return nil
	}

	if showHelp {
		printHelp()
		return nil
	}

	// Everything after flags is the command to run
	command := flag.Args()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Parse Cloud Run config
	cfg, err := config.Parse(configFile)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Resolve environment variables
	resolver, err := env.NewResolver(ctx, cfg)
	if err != nil {
		return fmt.Errorf("create env resolver: %w", err)
	}
	defer func() {
		if err := resolver.Cleanup(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: cleanup failed: %v\n", err)
		}
	}()

	envVars, err := resolver.Resolve(ctx)
	if err != nil {
		return fmt.Errorf("resolve environment: %w", err)
	}

	// If no command provided, print environment variables
	if len(command) == 0 {
		for _, envVar := range envVars {
			fmt.Println(envVar)
		}
		return nil
	}

	// Execute command with environment
	//nolint:gosec // looks insecure, but that's kind of the point
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)

	// Inherit existing environment variables
	cmd.Env = append(envVars, os.Environ()...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("execute command: %w", err)
	}

	return nil
}

func printHelp() {
	fmt.Println(`cloudrun-local - Run Cloud Run services locally with proper service account impersonation

USAGE:
    cloudrun-local [FLAGS] [-- COMMAND [ARGS...]]

FLAGS:
    -c, --config <file>    Path to Cloud Run service YAML config file (default: service.yaml)
    -h, --help             Show this help message
    -v, --version          Show version information

EXAMPLES:
    # Print environment variables
    cloudrun-local --config=service.yaml

    # Run a Go service with the environment
    cloudrun-local -c service.yaml -- go run ./cmd/server

    # Run with default config file (service.yaml)
    cloudrun-local -- npm start

    # Save environment to a file
    cloudrun-local > .env

DESCRIPTION:
    cloudrun-local reads a Cloud Run service configuration YAML file, impersonates
    the configured service account using your local gcloud credentials, resolves
    environment variables (including secrets from Secret Manager), and either prints
    them or executes a command with that environment.

    The tool requires:
    - Authenticated gcloud CLI (gcloud auth application-default login)
    - Permissions to impersonate the service account
    - Access to secrets referenced in the configuration

    Environment variables are resolved with the following priority (highest first):
    1. Current shell environment (allows overriding config values)
    2. Cloud Run configuration YAML
    3. Automatic variables (K_SERVICE, K_REVISION, etc.)

CONFIGURATION:
    The service account is read from: spec.template.spec.serviceAccountName
    The project ID is extracted from the service account email
    Environment variables are read from: spec.template.spec.containers[0].env`)
}
