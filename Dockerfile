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
RUN apk add --no-cache ca-certificates git
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/ptium ./cmd/ptium

FROM alpine:3.21
RUN apk add --no-cache ca-certificates nginx tini tzdata \
    && addgroup -S ptium \
    && adduser -S -D -H -G ptium ptium \
    && mkdir -p /app/web /tmp/nginx/client_body /tmp/nginx/proxy /tmp/nginx/fastcgi /tmp/nginx/uwsgi /tmp/nginx/scgi \
    && chown -R ptium:ptium /app /tmp/nginx
COPY --from=api-build /out/ptium /app/ptium
COPY --from=web-build /src/web/dist/ /app/web/
COPY deploy/nginx.conf /etc/nginx/nginx.conf
COPY deploy/container-entrypoint.sh /app/container-entrypoint.sh
RUN chmod 0755 /app/container-entrypoint.sh
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="Ptium" \
      org.opencontainers.image.description="Self-hosted AI presentation workspace" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.source="https://github.com/hkjang/ptium"
ENV PTIUM_INTERNAL_HTTP_ADDR=127.0.0.1:8081
USER ptium
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/readyz >/dev/null || exit 1
ENTRYPOINT ["/sbin/tini", "--", "/app/container-entrypoint.sh"]
