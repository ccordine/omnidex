FROM golang:1.24.1-alpine@sha256:43c094ad24b6ac0546c62193baeb3e6e49ce14d3250845d166c77c25f64b0386 AS build

RUN apk add --no-cache bash build-base nodejs npm
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN ./scripts/build-ui.sh
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o /out/agent-core ./cmd/core

FROM docker/compose-bin:v5.1.4@sha256:88d82497d9be33710c959aeaad5e541de5aa41a36d733e04ab09ccce74fa6b4c AS compose

FROM docker:29.5.1-cli@sha256:b40b3737eb3bf588d25bb856d3564dd3f9fdb32ac2fc19ebe85cc58d761692a5

ARG APP_UID=1000
ARG APP_GID=1000
COPY --from=compose /docker-compose /usr/local/libexec/docker/cli-plugins/docker-compose
RUN apk add --no-cache git nodejs npm bubblewrap build-base \
    && addgroup -S -g "${APP_GID}" app \
    && adduser -S -D -h /home/app -u "${APP_UID}" -G app app \
    && docker --version | grep -Eq '^Docker version 29\.5\.1,' \
    && test "$(docker compose version --short)" = "5.1.4"
USER app
WORKDIR /app

COPY --from=build /out/agent-core /usr/local/bin/agent-core
COPY --from=build /src/migrations /usr/local/migrations
COPY --from=build /usr/local/go /usr/local/go

ENV LISTEN_ADDR=:8090
ENV DOCKER_COMPOSE_VERSION=5.1.4
# docker compose loads .env via env_file; a host PATH (e.g. for mise/node on the
# bridge) must not replace this — agent-core lives in /usr/local/bin.
ENV PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV HOME=/home/app
EXPOSE 8090

# Absolute path so startup does not depend on PATH (see troubleshooting in README).
ENTRYPOINT []
CMD ["/usr/local/bin/agent-core"]
