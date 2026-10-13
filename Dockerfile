# Stage 1 — build the React frontend on the build host (no emulation).
FROM --platform=$BUILDPLATFORM node:20-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2 — cross-compile the Go binary for the target platform.
FROM --platform=$BUILDPLATFORM golang:1.26 AS go
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/assets/dist ./internal/assets/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-s -w -X main.version=$VERSION" -o /sealane ./cmd/server

# Stage 3 — minimal non-root runtime.
FROM gcr.io/distroless/static:nonroot
COPY --from=go /sealane /sealane
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/sealane"]
