FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ARG VITE_API_BASE_URL=/api/v1
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}
RUN npm run build

FROM golang:1.24-alpine AS api-build
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/ptium ./cmd/ptium

# The runtime is one static binary plus the workspace it serves itself: no
# reverse proxy, no shell entrypoint and no bundled database. Point
# DATABASE_URL at PostgreSQL and publish port 8080.
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 65532 -S ptium \
    && adduser -u 65532 -S -D -H -G ptium ptium \
    && mkdir -p /app/web \
    && chown -R ptium:ptium /app
COPY --from=api-build /out/ptium /app/ptium
COPY --from=web-build /src/web/dist/ /app/web/
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="Ptium" \
      org.opencontainers.image.description="Self-hosted AI presentation workspace that generates into your own PowerPoint templates" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.source="https://github.com/hkjang/ptium"
ENV HTTP_ADDR=:8080 \
    WEB_DIR=/app/web
USER ptium
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/readyz >/dev/null || exit 1
ENTRYPOINT ["/app/ptium"]
