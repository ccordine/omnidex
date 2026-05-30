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
EXPOSE 8090

CMD ["/usr/local/bin/agent-core"]
