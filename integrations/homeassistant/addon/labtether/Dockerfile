ARG BUILD_FROM=ghcr.io/home-assistant/amd64-base:3.19

FROM golang:1.26-alpine AS builder
ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH:-amd64} \
  go build -trimpath -ldflags="-s -w -X main.version=ha-addon" -o /out/labtether ./cmd/labtether

FROM ${BUILD_FROM}

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN apk add --no-cache \
  bash \
  ca-certificates \
  jq \
  postgresql16 \
  postgresql16-client \
  su-exec

COPY --from=builder /out/labtether /usr/local/bin/labtether
COPY integrations/homeassistant/addon/labtether/run.sh /run.sh
RUN chmod +x /run.sh

CMD ["/run.sh"]
