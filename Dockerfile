ARG OMNIDEX_COMMIT

FROM golang:1.24.1-alpine@sha256:43c094ad24b6ac0546c62193baeb3e6e49ce14d3250845d166c77c25f64b0386 AS build

ARG OMNIDEX_COMMIT

RUN apk add --no-cache bash build-base nodejs npm
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN case "${#OMNIDEX_COMMIT}" in 40|64) ;; *) echo "OMNIDEX_COMMIT must be exactly 40 or 64 lowercase hexadecimal characters" >&2; exit 1 ;; esac \
    && case "${OMNIDEX_COMMIT}" in *[!0123456789abcdef]*) echo "OMNIDEX_COMMIT must be exactly 40 or 64 lowercase hexadecimal characters" >&2; exit 1 ;; esac
RUN ./scripts/build-ui.sh
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-X github.com/gryph/omnidex/internal/version.Commit=${OMNIDEX_COMMIT}" \
    -o /out/agent-core ./cmd/core

FROM docker/compose-bin:v5.1.4@sha256:88d82497d9be33710c959aeaad5e541de5aa41a36d733e04ab09ccce74fa6b4c AS compose

FROM docker:29.5.1-cli@sha256:b40b3737eb3bf588d25bb856d3564dd3f9fdb32ac2fc19ebe85cc58d761692a5

ARG APP_UID=1000
ARG APP_GID=1000
ARG OMNIDEX_COMMIT
LABEL org.opencontainers.image.revision="${OMNIDEX_COMMIT}"
COPY --from=compose /docker-compose /usr/local/libexec/docker/cli-plugins/docker-compose
COPY scripts/initialize-runtime-volumes.sh /usr/local/bin/initialize-omnidex-volumes
RUN validate_host_id() { \
        label="$1"; value="$2"; \
        case "${value}" in ""|0|0*|*[!0123456789]*) echo "${label} must be one exact positive numeric host identity" >&2; return 1 ;; esac; \
        if [ "${#value}" -gt 10 ] || [ "${value}" -gt 4294967294 ]; then echo "${label} must be one exact positive numeric host identity" >&2; return 1; fi; \
    }; \
    validate_host_id APP_UID "${APP_UID}" \
    && validate_host_id APP_GID "${APP_GID}" \
    && apk add --no-cache git nodejs npm bubblewrap build-base \
    && addgroup -S -g "${APP_GID}" app \
    && adduser -S -D -h /home/app -u "${APP_UID}" -G app app \
    && mkdir -p /var/lib/omnidex-deployment /var/cache/omnidex/gomod \
    && chown -R app:app /var/lib/omnidex-deployment /var/cache/omnidex/gomod \
    && chmod 0755 /usr/local/bin/initialize-omnidex-volumes \
    && docker --version | grep -Eq '^Docker version 29\.5\.1,' \
    && test "$(docker compose version --short)" = "5.1.4"
USER ${APP_UID}:${APP_GID}
WORKDIR /app

COPY --from=build /out/agent-core /usr/local/bin/agent-core
COPY --from=build /src/database/setup.sql /usr/local/database/setup.sql
COPY --from=build /usr/local/go /usr/local/go

ENV LISTEN_ADDR=:8090
ENV DOCKER_COMPOSE_VERSION=5.1.4
# The core binary is built with -trimpath, so go/importer cannot recover the
# copied toolchain root from its build metadata. Keep the runtime toolchain
# location explicit for deterministic standard-library type checking.
ENV GOROOT=/usr/local/go
# docker compose loads .env via env_file; a host PATH (e.g. for mise/node on the
# bridge) must not replace this — agent-core lives in /usr/local/bin.
ENV PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV HOME=/home/app
EXPOSE 8090

# Absolute path so startup does not depend on PATH (see troubleshooting in README).
ENTRYPOINT []
CMD ["/usr/local/bin/agent-core"]
