# App Store API

Backend API for managing Helm chart installations on a K3s cluster, simulating an app store experience.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Project Structure](#project-structure)
- [Configuration](#configuration)
- [Getting Started](#getting-started)
    - [Local Development](#local-development)
- [Building](#building)
- [Docker](#docker)
- [API Endpoints](#api-endpoints)
- [Kubernetes Deployment](#kubernetes-deployment)
- [Contributing](#contributing)

## Prerequisites

- Go (version specified in `go.mod`, e.g., 1.24)
- Docker
- kubectl
- Helm CLI (v3+)
- A running Kubernetes cluster (e.g., K3s, Minikube, Docker Desktop K8s)
- Access to `charts.yaml` (local or via ConfigMap)

## Project Structure

- `cmd/appstore/main.go`: Main application entry point.
- `pkg/`: Contains core packages:
    - `api/`: HTTP handlers, routes, middleware.
    - `appcatalog/`: Logic for managing the chart catalog.
    - `config/`: Application configuration management.
    - `helm/`: Helm and Kubernetes client interaction logic.
- `charts.yaml`: Defines the list of available Helm charts for the store.
- `Dockerfile`: For building the application Docker image.
- `k8s/`: Kubernetes manifest files for deployment.

## Configuration

The application is configured via environment variables. See `pkg/config/config.go` for details. Key variables:

- `APP_PORT`: Port the API listens on (default: `8080`).
- `GIN_MODE`: Gin framework mode (`debug` or `release`, default: `debug`).
- `APP_INSTALL_NAMESPACE`: Kubernetes namespace where charts will be installed (default: `app-store-apps`).
- `KUBECONFIG`: Path to the kubeconfig file (tries in-cluster config first, then default path).
- `HELM_DRIVER`: Helm storage driver (default: `secret`).
- `HELM_TIMEOUT_SECONDS`: Timeout for Helm operations (default: `300`).
- `CHART_CONFIG_PATH`: Path to the chart catalog definition file (default: `charts.yaml`).

The `charts.yaml` file at the root (or specified by `CHART_CONFIG_PATH`) defines the applications available in the
store.

## Getting Started

### Local Development

1. **Clone the repository:**
   ```bash
   git clone https://github.com/YOUR_USERNAME/app-store-api.git
   cd app-store-api
   ```

2. **Ensure KUBECONFIG is set:**
   Make sure your `KUBECONFIG` environment variable points to a valid Kubernetes cluster, or that `~/.kube/config` is
   set up. The API needs to interact with a K8s cluster.

3. **Install dependencies:**
   ```bash
   go mod tidy
   ```

4. **Run the application:**
   ```bash
   go run ./cmd/appstore/main.go
   ```
   The API will start, typically on port 8080. Check logs for the exact address and any initialization errors.

   For local development, you might want to set `GIN_MODE=debug`.

## Building

To build the binary:

```bash
go build -o app-store-api ./cmd/appstore/main.go
```

## Docker

To build the Docker image:

```bash
docker build -t app-store-api:latest .
```

To run the Docker image (example, assuming KUBECONFIG is mounted and network configured):

```bash
docker run -p 8080:8080 \
-v ~/.kube/config:/root/.kube/config:ro \
--env APP_INSTALL_NAMESPACE=my-apps \
app-store-api:latest
```

*Note: For running locally with Docker, ensure the Kubernetes context in the mounted kubeconfig points to a reachable
cluster from the Docker container's perspective (e.g., host.docker.internal or an external IP).*

## API Endpoints

(Refer to `pkg/api/routes.go` for detailed routes)

- `GET /health`: Health check.
- `GET /api/charts`: List available charts.
- `POST /api/charts/:chartName/install`: Install a chart.
    - Body (JSON, optional): `{"release_name": "custom-name", "values": {"key": "value"}}`
- `GET /api/releases`: List installed releases.
- `GET /api/releases/:releaseName/status`: Get status of a specific release.
- `DELETE /api/releases/:releaseName`: Uninstall a release.

## Kubernetes Deployment

Manifests for deploying the API to Kubernetes are located in the k8s/ directory.

1. Build and push the Docker image to a registry accessible by your K8s cluster (e.g., GHCR, Docker Hub).
   Example for GHCR: `ghcr.io/YOUR_USERNAME/app-store-api:tag`
2. Update the `image` field in `k8s/02-deployment.yaml` to point to your pushed image.
3. Apply the manifests:

```bash
kubectl apply -f k8s/00-namespaces.yaml
kubectl apply -f k8s/01-rbac.yaml
# If using charts.yaml via ConfigMap:
# kubectl create configmap chart-config --from-file=charts.yaml -n app-store-api
kubectl apply -f k8s/02-deployment.yaml
kubectl apply -f k8s/03-service.yaml
```

## Contributing

Please follow Conventional Commits specification for commit messages to enable automated versioning and changelog
generation.