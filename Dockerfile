FROM golang:1.24.1-alpine AS build

RUN apk add --no-cache bash build-base nodejs npm
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN ./scripts/build-ui.sh
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/agent-core ./cmd/core

FROM alpine:3.20

ARG APP_UID=1000
ARG APP_GID=1000
RUN apk add --no-cache git go nodejs npm \
    && addgroup -S -g "${APP_GID}" app \
    && adduser -S -D -h /home/app -u "${APP_UID}" -G app app
USER app
WORKDIR /app

COPY --from=build /out/agent-core /usr/local/bin/agent-core
COPY --from=build /src/migrations /usr/local/migrations

ENV LISTEN_ADDR=:8090
# docker compose loads .env via env_file; a host PATH (e.g. for mise/node on the
# bridge) must not replace this — agent-core lives in /usr/local/bin.
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV HOME=/home/app
EXPOSE 8090

# Absolute path so startup does not depend on PATH (see troubleshooting in README).
CMD ["/usr/local/bin/agent-core"]
