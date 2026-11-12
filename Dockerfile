# ---------- build stage ----------
FROM golang:1.22-alpine AS build

WORKDIR /src

# cache go modules
COPY go.mod go.sum* ./
RUN go mod download

# build binary tĩnh
COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags='-s -w' \
    -o /out/glitch .

# ---------- runtime stage ----------
FROM alpine:3.20

# font emoji outline (Go render được) + font cơ bản
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    fontconfig \
    ttf-dejavu \
    font-noto-emoji

# font-noto-emoji trên Alpine có cả bản outline + color
# → kiểm tra path sau khi cài, thường là:
#   /usr/share/fonts/noto/NotoColorEmoji.ttf      (bitmap, Go skip)
#   /usr/share/fonts/noto/NotoEmoji-Regular.ttf   (outline, Go dùng)

WORKDIR /app

COPY --from=build /out/glitch /app/glitch

# cache + fonts dir (mount volume để persist cache nếu cần)
RUN mkdir -p /app/cache /app/fonts

ENV PORT=8080 \
    CACHE_DIR=/app/cache \
    FONTS_DIR=/app/fonts

EXPOSE 8080

# healthcheck nhẹ (Render có health check riêng)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://0.0.0.0:8080/fonts >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/glitch"]
