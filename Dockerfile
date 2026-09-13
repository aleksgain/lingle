# syntax=docker/dockerfile:1

# ---- build -----------------------------------------------------------
FROM golang:1.24-alpine AS build

# cgo is needed for the SQLite driver; musl-dev lets us link it statically.
RUN apk add --no-cache build-base

WORKDIR /src

# Dependencies first so edits to the source do not invalidate this layer.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=1 go build \
        -tags "sqlite_omit_load_extension" \
        -ldflags "-s -w -X main.version=${VERSION} -linkmode external -extldflags '-static'" \
        -o /out/lingle ./cmd/lingle

# ---- runtime ---------------------------------------------------------
FROM alpine:3.20

LABEL org.opencontainers.image.title="Lingle" \
      org.opencontainers.image.description="Self-hosted word guessing game with switchable languages" \
      org.opencontainers.image.source="https://github.com/aleksgain/lingle" \
      org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache su-exec

COPY --from=build /out/lingle /usr/local/bin/lingle
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENV LINGLE_ADDR=":8080" \
    LINGLE_DB="/data/lingle.db" \
    LINGLE_DEFAULT_LANG="ru" \
    LINGLE_LANGUAGES="" \
    LINGLE_TITLE="" \
    PUID=99 \
    PGID=100

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
