# Stage 1 — build the React frontend on the build host (no emulation).
# Base images are pinned by digest (Dependabot proposes updates), so a rebuild of the same commit
# gives the image that was scanned.
FROM --platform=$BUILDPLATFORM node:26-alpine@sha256:c6ad9e9c1d80f6eb607c26b4b0a132ff7a2344fad1c78ea3baa2a7d4c8de6d94 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build
COPY hack/npm-notices.mjs /tmp/
RUN node /tmp/npm-notices.mjs > /tmp/npm-notices

# Stage 2 — cross-compile the Go binary for the target platform.
FROM --platform=$BUILDPLATFORM golang:1.27@sha256:2920bb124f1a2e12a1d39f0cab756ae0068441ee1da7655dec7f5e374944095b AS go
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
FROM gcr.io/distroless/static:nonroot@sha256:25ac8f0c3b2334a9eb05452266bd4697864a8c8ef0ef71e5e6480d7f92868329
COPY --from=go /sealane /sealane
COPY LICENSE /licenses/LICENSE
COPY --from=go /THIRD_PARTY_NOTICES /licenses/THIRD_PARTY_NOTICES
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/sealane"]
