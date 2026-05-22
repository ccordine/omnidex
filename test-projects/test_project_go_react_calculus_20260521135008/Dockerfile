# Stage 1: Build the Go backend
FROM golang:latest AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Stage 2: Build the React frontend
FROM node:14 as react-build
WORKDIR /app/client
COPY client/package.json client/yarn.lock ./
RUN yarn install
COPY client/src ./src
COPY client/public ./public
RUN yarn build

# Stage 3: Final stage with Go backend and React frontend
FROM nginx:alpine
COPY --from=builder /app/main /usr/local/bin/app
COPY --from=react-build /app/client/build /usr/share/nginx/html
EXPOSE 80
CMD ["/usr/local/bin/app"]
