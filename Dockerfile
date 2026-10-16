FROM node:22-alpine AS panel
WORKDIR /src
COPY apps/panel/package.json apps/panel/package-lock.json ./
RUN npm ci
COPY apps/panel/ ./
RUN npm run build

FROM golang:1.26-alpine AS server
ARG VERSION=0.1.0
WORKDIR /src
COPY apps/server/go.mod apps/server/go.sum ./
RUN go mod download
COPY apps/server/ ./
COPY --from=panel /src/dist ./internal/panel/dist
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X harbor/internal/config.Version=${VERSION}" -o /harbor ./cmd/harbor

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=server /harbor /usr/local/bin/harbor
ENV HARBOR_ADDR=:8787
ENV HARBOR_DATA_DIR=/data
VOLUME /data
EXPOSE 8787
ENTRYPOINT ["harbor", "serve"]
