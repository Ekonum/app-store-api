# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

# Install build dependencies: git (for go get), curl & bash (for Helm script), openssl (for Helm script checksum)
RUN apk add --no-cache git curl bash openssl

# Install Helm CLI into the builder stage
# This Helm version will be used by the application if it shells out to 'helm' command.
# The Go Helm SDK client is also compiled into the app.
RUN curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash

WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify

# Copy the entire application source code
COPY . .

# Build the application
# Output binary to /app/app-store-api inside the builder stage
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/app-store-api ./cmd/appstore/main.go


# Stage 2: Create the final lightweight image
FROM alpine:latest

# Install runtime dependencies: kubectl (for namespace creation by app, can be removed if ns is pre-created)
# Helm CLI (if application shells out to it, good to have same version as build)
# ca-certificates for HTTPS calls by the app or Helm SDK.
RUN apk add --no-cache kubectl helm ca-certificates

WORKDIR /app

# Copy the compiled application binary from the builder stage
COPY --from=builder /app/app-store-api /app/app-store-api
# Copy the Helm binary from the builder stage (ensures version consistency if app shells out)
COPY --from=builder /usr/local/bin/helm /usr/local/bin/helm

# Copy chart configuration (if it's part of the image, otherwise use a ConfigMap)
COPY charts.yaml /app/charts.yaml

# Expose the port the API listens on (should match APP_PORT env var)
EXPOSE 8080

# Set default environment variables (can be overridden at runtime)
ENV APP_PORT="8080"
ENV GIN_MODE="release"
ENV APP_INSTALL_NAMESPACE="app-store-apps"
ENV CHART_CONFIG_PATH="/app/charts.yaml"
# KUBECONFIG will be automatically mounted for the ServiceAccount in Kubernetes.
# HELM_DRIVER default is "secret"

# Command to run the application
ENTRYPOINT ["/app/app-store-api"]