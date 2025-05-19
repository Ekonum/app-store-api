# Étape 1: Build de l'application Go
FROM golang:1.24-alpine AS builder

# Installer git (nécessaire pour go get) et Helm
RUN apk add --no-cache git curl bash openssl

# Installation de Helm
RUN curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Construire l'application Go
# Désactiver CGO pour une compilation statique si possible (pas toujours le cas avec k8s client-go)
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app-store-api .

# Étape 2: Créer l'image finale légère
FROM alpine:latest

# Installer kubectl et Helm (et ca-certificates pour les connexions HTTPS)
RUN apk add --no-cache kubectl helm ca-certificates

WORKDIR /app

# Copier le binaire de l'application depuis l'étape de build
COPY --from=builder /app/app-store-api .
# Copier le binaire helm (s'il n'est pas dans le PATH standard d'alpine ou si version spécifique)
COPY --from=builder /usr/local/bin/helm /usr/local/bin/helm


# Le binaire kubectl est déjà dans le PATH grâce à apk add

# Exposer le port sur lequel l'API écoute
EXPOSE 8080

# Commande pour exécuter l'application
# Le KUBECONFIG sera automatiquement monté par Kubernetes pour le ServiceAccount
ENTRYPOINT ["/app/app-store-api"]