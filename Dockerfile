# Stage 1 — build the React frontend (Vite outputs to internal/assets/dist).
FROM node:20-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2 — build the Go binary, embedding the built frontend.
FROM golang:1.26 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/assets/dist ./internal/assets/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /sealane ./cmd/server

# Stage 3 — minimal non-root runtime.
FROM gcr.io/distroless/static:nonroot
COPY --from=go /sealane /sealane
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/sealane"]
