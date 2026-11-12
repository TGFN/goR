# ---------- build stage ----------
FROM golang:1.22-alpine AS build

WORKDIR /src

# copy source trước, không copy go.sum riêng
COPY go.mod ./
COPY main.go ./

# -mod=mod: cho phép go build tự thêm entry vào go.mod/go.sum
RUN CGO_ENABLED=0 GOOS=linux go build \
    -mod=mod \
    -trimpath \
    -ldflags='-s -w' \
    -o /out/glitch .

# ---------- runtime stage ----------
FROM alpine:3.20

# Bỏ font-noto-emoji (chỉ có bitmap — Go không render được)
# Cài wget để tải NotoEmoji-Regular.ttf (outline — Go render được)
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    fontconfig \
    ttf-dejavu \
    wget \
 && mkdir -p /usr/share/fonts/noto \
 && wget -qO /usr/share/fonts/noto/NotoEmoji-Regular.ttf \
      https://github.com/googlefonts/noto-emoji/raw/main/fonts/NotoEmoji-Regular.ttf \
 && fc-cache -f

WORKDIR /app

COPY --from=build /out/glitch /app/glitch

RUN mkdir -p /app/cache /app/fonts

ENV PORT=8080 \
    CACHE_DIR=/app/cache \
    FONTS_DIR=/app/fonts

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/fonts >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/glitch"]
