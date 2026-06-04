# --- Stage 1: build frontend ---
FROM node:24-alpine AS frontend
WORKDIR /fe
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# --- Stage 2: build backend ---
FROM golang:1.26-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# --- Stage 3: runtime ---
FROM alpine:3.22
RUN adduser -D -H -u 10001 appuser && apk add --no-cache ca-certificates tzdata wget
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=frontend /fe/dist /app/web
USER appuser
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --retries=3 CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["/app/api"]
