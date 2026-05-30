FROM golang:1.24.1-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/agent-core ./cmd/core

FROM alpine:3.20

RUN addgroup -S app && adduser -S app -G app
USER app
WORKDIR /app

COPY --from=build /out/agent-core /usr/local/bin/agent-core
COPY --from=build /src/skills ./skills
COPY --from=build /src/migrations ./migrations
COPY --from=build /src/database ./database

ENV LISTEN_ADDR=:8090
# docker compose loads .env via env_file; a host PATH (e.g. for mise/node on the
# bridge) must not replace this — agent-core lives in /usr/local/bin.
ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
EXPOSE 8090

# Absolute path so startup does not depend on PATH (see troubleshooting in README).
CMD ["/usr/local/bin/agent-core"]
