# Stage 1: build all Go binaries
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG TARGETOS TARGETARCH
RUN for bin in server migrate worker thumbgen admin; do \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
        go build -ldflags="-s -w" -o /out/$bin ./cmd/$bin || exit 1; \
    done

# Stage 2: runtime (ffmpeg is used by thumbgen / thumbnail extraction)
FROM alpine:3.20
RUN apk add --no-cache ca-certificates ffmpeg wget
WORKDIR /app

COPY --from=go-builder /out/ /app/
COPY migrations /app/migrations

EXPOSE 8081
HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:8081/health/ready || exit 1

CMD ["/app/server"]
