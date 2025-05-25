# ---------- ビルドステージ ----------
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o audio-probe-ffmpeg ./cmd/audio-probe-ffmpeg/main.go

# ---------- 実行ステージ ----------
FROM alpine:3.20
RUN apk add --no-cache ffmpeg
WORKDIR /app
COPY --from=builder /app/audio-probe-ffmpeg ./audio-probe-ffmpeg
ENTRYPOINT ["/app/audio-probe-ffmpeg"]
