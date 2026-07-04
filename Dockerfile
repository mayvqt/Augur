# syntax=docker/dockerfile:1.7

FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/augur ./cmd/augur

FROM alpine:3.21
LABEL org.opencontainers.image.source="https://github.com/mayvqt/Augur" \
      org.opencontainers.image.title="Augur" \
      org.opencontainers.image.description="Discord request bot for Seerr"
RUN apk add --no-cache ca-certificates shadow su-exec tzdata \
    && addgroup -g 1000 augur \
    && adduser -D -H -u 1000 -G augur augur \
    && mkdir -p /data /app \
    && chown -R augur:augur /data /app
WORKDIR /app
COPY --from=build /out/augur /usr/local/bin/augur
COPY config.docker.json /app/config.docker.json
COPY --chmod=755 docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
ENV AUGUR_CONFIG=/data/config.json \
    AUGUR_STORAGE_PATH=/data/augur-state.json \
    PUID=99 \
    PGID=100
VOLUME ["/data"]
STOPSIGNAL SIGTERM
ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["augur"]
