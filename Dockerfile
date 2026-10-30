# Stage 1 — build the React frontend on the build host (no emulation).
# Base images are pinned by digest (Dependabot proposes updates), so a rebuild of the same commit
# gives the image that was scanned.
FROM --platform=$BUILDPLATFORM node:25-alpine@sha256:bdf2cca6fe3dabd014ea60163eca3f0f7015fbd5c7ee1b0e9ccb4ced6eb02ef4 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build
COPY hack/npm-notices.mjs /tmp/
RUN node /tmp/npm-notices.mjs > /tmp/npm-notices

# Stage 2 — cross-compile the Go binary for the target platform.
FROM --platform=$BUILDPLATFORM golang:1.26@sha256:670bb5aa5349ae03b8516764718486f4f16399134bdc9acd0c46a23f043ab05c AS go
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
FROM gcr.io/distroless/static:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99
COPY --from=go /sealane /sealane
COPY LICENSE /licenses/LICENSE
COPY --from=go /THIRD_PARTY_NOTICES /licenses/THIRD_PARTY_NOTICES
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/sealane"]
