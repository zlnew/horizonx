# Production image for the HorizonX control-plane server.
# Multi-stage: builds the unified `horizonx` binary (server + agent + setup)
# and runs `horizonx server`. Config comes from env/.env at runtime.
#
# Build (used by the release workflow):
#   docker build -t ghcr.io/zlnew/horizonx:<tag> .
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static binary; version stamped from the git tag (e.g. v0.4.0).
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X horizonx/internal/version.Version=${VERSION}" -o /out/horizonx ./cmd/horizonx

FROM alpine:3.20
RUN apk add --no-cache ca-certificates git docker-cli docker-cli-compose curl bash
# Agent needs a docker CLI + compose to run deployments on the host.
RUN mkdir -p /var/lib/horizonx/apps /etc/horizonx
COPY --from=build /out/horizonx /usr/local/bin/horizonx
WORKDIR /etc/horizonx
EXPOSE 80
ENTRYPOINT ["horizonx"]
CMD ["server"]
