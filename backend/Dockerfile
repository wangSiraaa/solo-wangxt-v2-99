FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -u 10001 appuser
COPY --from=build /server /app/server
COPY --from=build /src/migrations /app/migrations
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/server"]
