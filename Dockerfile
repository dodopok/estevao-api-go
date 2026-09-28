# syntax=docker/dockerfile:1
# Go server, worker and seeder in one image (the same image serves the web and
# the worker service, as the Rails image does). tzdata is required: user time
# zones and the country inferred from a zone are read from /usr/share/zoneinfo.

FROM golang:1.24-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/estevao-api ./cmd/estevao-api && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/estevao-worker ./cmd/estevao-worker && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/estevao-seed ./cmd/estevao-seed

FROM debian:bookworm-slim
RUN apt-get update -qq && \
    apt-get install --no-install-recommends -y ca-certificates tzdata curl && \
    rm -rf /var/lib/apt/lists /var/cache/apt/archives && \
    useradd --system --uid 1000 --create-home app
WORKDIR /app
COPY --from=build /out/ /usr/local/bin/
COPY public ./public
COPY db/schema.sql ./db/schema.sql
COPY seeds ./seeds
ENV RAILS_ENV=production PORT=3000 PUBLIC_DIR=/app/public
USER app
EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s CMD curl -fsS "http://127.0.0.1:${PORT}/up" || exit 1
CMD ["estevao-api"]
