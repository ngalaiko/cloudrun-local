# cloudrun-local

Run Google Cloud Run services and jobs locally with proper service account impersonation and environment configuration.

## Features

- Service account impersonation using application default credentials
- Secret resolution from Google Secret Manager
- Environment variable resolution from Cloud Run YAML configuration
- Support for both Cloud Run Services and Jobs
- No gcloud CLI dependency for runtime operations

## Prerequisites

- Application default credentials configured (`gcloud auth application-default login`)
- IAM permissions to impersonate the service account
- Access to secrets referenced in the configuration

## Installation

```bash
go install github.com/ngalaiko/cloudrun-local/cmd/cloudrun-local@latest
```

Or build from source:

```bash
git clone <repository-url>
cd cloudrun-local
go build -o cloudrun-local ./cmd/cloudrun-local
```

## Usage

Print environment variables:

```bash
cloudrun-local -c service.yaml
```

Execute command with environment:

```bash
cloudrun-local -c service.yaml -- go run ./cmd/server
```

Export to file:

```bash
cloudrun-local -c service.yaml > .env
```

### Options

```
-c, --config <file>    Path to Cloud Run YAML config (default: service.yaml)
-h, --help             Show help
-v, --version          Show version
```

## Configuration Format

### Cloud Run Service

```yaml
apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: my-service
spec:
  template:
    spec:
      serviceAccountName: my-account@my-project.iam.gserviceaccount.com
      containers:
      - image: gcr.io/my-project/my-service
        env:
        - name: DATABASE_URL
          value: "postgres://localhost:5432/db"
        - name: API_KEY
          valueFrom:
            secretKeyRef:
              name: api-key
              key: latest
```

### Cloud Run Job

```yaml
apiVersion: run.googleapis.com/v1
kind: Job
metadata:
  name: my-job
spec:
  template:
    spec:
      template:
        spec:
          serviceAccountName: my-account@my-project.iam.gserviceaccount.com
          containers:
          - image: gcr.io/my-project/my-job
            env:
            - name: TASK_QUEUE
              value: "default"
            - name: SECRET_TOKEN
              valueFrom:
                secretKeyRef:
                  name: token
                  key: latest
```

### Automatic Environment Variables

The following variables are automatically set:

- `K_SERVICE`: Service/job name
- `K_REVISION`: Set to "local"
- `GOOGLE_CLOUD_PROJECT`: Extracted from service account email
- `GOOGLE_APPLICATION_CREDENTIALS`: Path to temporary credentials file

## How It Works

1. Parse the Cloud Run YAML configuration
2. Impersonate the service account using application default credentials
3. Fetch secrets from Secret Manager with impersonated credentials
4. Resolve all environment variables
5. Print variables or execute command with environment

## Environment Variable Priority

Environment variables are resolved in the following priority order (highest to lowest):

1. **Current shell environment** - Variables from your current shell session
2. **Cloud Run configuration** - Variables defined in the YAML config file
3. **Automatic variables** - System-set variables (K_SERVICE, K_REVISION, etc.)

This means you can override any variable from the config by setting it in your shell:

```bash
# Override DATABASE_URL from config
export DATABASE_URL="postgres://localhost:5433/testdb"
cloudrun-local -c service.yaml -- go run ./cmd/server

# Override secrets temporarily
API_KEY=test-key cloudrun-local -c service.yaml -- npm test
```

## Examples

Run a Go service:

```bash
cloudrun-local -c service.yaml -- go run ./cmd/server
```

Run a Cloud Run job:

```bash
cloudrun-local -c job.yaml -- go run ./cmd/worker
```

Use with Docker:

```bash
cloudrun-local -c service.yaml > .env
docker run --env-file .env my-image
```

Check environment variables:

```bash
cloudrun-local -c service.yaml | grep DATABASE_URL
```

## Troubleshooting

**No application default credentials found**

```bash
gcloud auth application-default login
```

**Failed to generate access token**

Grant the `iam.serviceAccountTokenCreator` role:

```bash
gcloud iam service-accounts add-iam-policy-binding SERVICE_ACCOUNT_EMAIL \
  --member="user:YOUR_EMAIL" \
  --role="roles/iam.serviceAccountTokenCreator"
```

**Unable to access secret**

Grant the service account access to secrets:

```bash
gcloud secrets add-iam-policy-binding SECRET_NAME \
  --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
  --role="roles/secretmanager.secretAccessor"
```

## Security

- Temporary credential files are created with `0600` permissions
- Files are automatically cleaned up on exit
- Requires explicit IAM permissions for service account impersonation

## Acknowledgments

Thanks to [einride/sage](https://github.com/einride/sage) for inspiration and ideas.

## License

MIT
