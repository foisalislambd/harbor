FROM node:22-alpine AS panel
WORKDIR /src
COPY apps/panel/package.json apps/panel/package-lock.json ./
RUN npm ci
COPY apps/panel/ ./
RUN npm run build

FROM golang:1.26-alpine AS server
WORKDIR /src
COPY apps/server/go.mod apps/server/go.sum ./
RUN go mod download
COPY apps/server/ ./
RUN CGO_ENABLED=0 go build -o /harbor ./cmd/harbor

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=server /harbor /usr/local/bin/harbor
COPY --from=panel /src/dist /panel
ENV HARBOR_ADDR=:8787
ENV HARBOR_DATA_DIR=/data
ENV HARBOR_PANEL_DIR=/panel
VOLUME /data
EXPOSE 8787
ENTRYPOINT ["harbor"]
