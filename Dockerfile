# syntax=docker/dockerfile:1

# Builder stages run on the build machine's platform and cross-compile for the
# target, so multi-architecture images build without CPU emulation.

# --- Web UI -------------------------------------------------------------------
FROM --platform=$BUILDPLATFORM node:24-alpine AS frontend
WORKDIR /src/frontend
# pnpm comes from the "packageManager" field in package.json.
RUN corepack enable
COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

# --- Server binary ------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
COPY --from=frontend /src/frontend/dist ./frontend/dist
ARG TARGETOS TARGETARCH
ARG VERSION=dev
ARG COMMIT=
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -tags ui -trimpath \
      -ldflags "-s -w -X github.com/Meidorislav/c4-forge/internal/buildinfo.Version=${VERSION} -X github.com/Meidorislav/c4-forge/internal/buildinfo.Commit=${COMMIT}" \
      -o /out/c4forge ./cmd/c4forge

# --- Runtime ------------------------------------------------------------------
# Static binary on distroless: no shell or package manager, runs as non-root.
FROM gcr.io/distroless/static-debian13:nonroot
ARG VERSION=dev
ARG COMMIT=
LABEL org.opencontainers.image.title="c4-forge" \
      org.opencontainers.image.description="Self-hosted C4 architecture modeling" \
      org.opencontainers.image.source="https://github.com/Meidorislav/c4-forge" \
      org.opencontainers.image.licenses="Apache-2.0" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}"
COPY --from=backend /out/c4forge /c4forge
ENV C4FORGE_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=10s --timeout=5s --start-period=40s --retries=3 \
  CMD ["/c4forge", "healthcheck"]
ENTRYPOINT ["/c4forge"]
