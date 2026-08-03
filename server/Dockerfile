# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/ptium ./cmd/ptium

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && addgroup -S ptium && adduser -S -G ptium ptium
COPY --from=build /out/ptium /usr/local/bin/ptium
USER ptium
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/ptium"]

