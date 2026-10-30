# Stage 1 — build the React frontend on the build host (no emulation).
FROM --platform=$BUILDPLATFORM node:20-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build
COPY hack/npm-notices.mjs /tmp/
RUN node /tmp/npm-notices.mjs > /tmp/npm-notices

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
# License texts of everything that ships in the image (also a release asset: --target notices).
COPY --from=web /tmp/npm-notices /tmp/
RUN { echo "Sealane (MIT, see LICENSE) includes the third-party software below, each under its own license."; \
      echo; hack/go-notices.sh; echo; cat /tmp/npm-notices; } > /THIRD_PARTY_NOTICES

FROM scratch AS notices
COPY --from=go /THIRD_PARTY_NOTICES /

# Stage 3 — minimal non-root runtime.
FROM gcr.io/distroless/static:nonroot
COPY --from=go /sealane /sealane
COPY LICENSE /licenses/LICENSE
COPY --from=go /THIRD_PARTY_NOTICES /licenses/THIRD_PARTY_NOTICES
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/sealane"]
