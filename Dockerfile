# syntax=docker/dockerfile:1

# SQLCipher uses CGO: build and run against the same Debian/glibc generation.
FROM golang:1.27.3-bookworm AS build
WORKDIR /src
ENV CGO_ENABLED=1 GOTOOLCHAIN=local

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
# The generated protobuf packages are large. Limit parallelism on small hosts.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOMAXPROCS=2 go build -p 2 -trimpath -ldflags="-s -w" -o /out/ ./cmd/solis ./cmd/makino

FROM debian:bookworm-slim AS runtime
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 solis \
    && useradd --uid 10001 --gid solis --no-create-home --shell /usr/sbin/nologin solis \
    && mkdir -p /app/resources /app/config \
    && chown solis:solis /app/resources /app/config
WORKDIR /app
ENV SOLIS_ENV=prd SOLIS_LOG_TYPE=1 SOLIS_RESOURCE_DIR=/app/resources
# Both programs ship together; run administrative commands with
# `docker compose exec solis /app/makino ...`.
COPY --from=build /out/solis /out/makino /app/
USER 10001:10001

# Default build target. Secrets, configuration, and game assets are mounted at
# runtime, never included in the image.
FROM runtime AS server
EXPOSE 50051 50052
ENTRYPOINT ["/app/solis"]
