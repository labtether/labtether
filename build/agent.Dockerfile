FROM golang:1.26-alpine AS builder

ARG SERVICE_DIR
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 \
    GOOS=${TARGETOS:-$(go env GOOS)} \
    GOARCH=${TARGETARCH:-$(go env GOARCH)} \
    go build -trimpath -ldflags="-s -w" -o /out/service ./${SERVICE_DIR}

# Agent needs a shell for PTY terminal sessions.
# Using Alpine instead of distroless so /bin/sh and /bin/bash are available.
FROM alpine:3.21
RUN apk add --no-cache \
    bash \
    ca-certificates \
    gstreamer \
    gst-plugins-base \
    gst-plugins-good \
    gst-plugins-bad \
    gst-plugins-ugly \
    gst-libav \
    xdotool
# Run as a non-root user. PTY sessions are proxied through the hub so root is not required.
RUN adduser -D -u 1001 agent
COPY --from=builder /out/service /service
USER agent
ENTRYPOINT ["/service"]
