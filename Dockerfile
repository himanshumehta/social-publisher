# Stage 1: build all Go binaries (static, no cgo)
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG TARGETOS TARGETARCH
RUN for bin in server migrate worker thumbgen admin; do \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
        go build -trimpath -ldflags="-s -w" -o /out/$bin ./cmd/$bin || exit 1; \
    done

# Stage 2: runtime. Alpine rather than distroless because thumbgen and the
# thumbnail service shell out to ffmpeg; the image adds nothing else.
FROM alpine:3.22
RUN apk add --no-cache ca-certificates ffmpeg wget \
 && addgroup -S -g 10001 app \
 && adduser -S -D -H -u 10001 -G app app
WORKDIR /app

COPY --from=go-builder /out/ /app/
COPY migrations /app/migrations

# Run as an unprivileged user; nothing in /app is writable by it.
USER 10001:10001

EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:8081/health/ready || exit 1

CMD ["/app/server"]
