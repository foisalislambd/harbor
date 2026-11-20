FROM node:22-alpine AS panel
WORKDIR /src
COPY apps/panel/package.json apps/panel/package-lock.json ./
RUN npm ci
COPY apps/panel/ ./
RUN npm run build

FROM golang:1.26-alpine AS server
ARG VERSION
WORKDIR /src
COPY apps/server/go.mod apps/server/go.sum ./
RUN go mod download
COPY apps/server/ ./
COPY VERSION.md /tmp/VERSION.md
COPY --from=panel /src/dist ./internal/panel/dist
RUN v="${VERSION}"; \
    if [ -z "$v" ]; then v="$(tr -d '[:space:]' < /tmp/VERSION.md)"; fi; \
    CGO_ENABLED=0 go build -ldflags "-s -w -X harbor/internal/config.Version=${v}" -o /harbor ./cmd/harbor

FROM alpine:3.22
RUN apk add --no-cache ca-certificates su-exec wget \
  && addgroup -S harbor \
  && adduser -S -G harbor -H -h /data harbor
COPY --from=server /harbor /usr/local/bin/harbor
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 755 /usr/local/bin/docker-entrypoint.sh /usr/local/bin/harbor
ENV HARBOR_ADDR=:8787
ENV HARBOR_DATA_DIR=/data
VOLUME /data
EXPOSE 8787
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8787/api/v1/health || exit 1
ENTRYPOINT ["docker-entrypoint.sh"]
