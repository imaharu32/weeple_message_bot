# Goのビルド環境
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 依存関係をコピー
COPY go.mod go.sum ./
RUN go mod download

# ソースコードをコピー
COPY . .

# ビルド
RUN go build -o bot .

# 実行用の軽量イメージ
FROM alpine:latest

WORKDIR /app

# 実行ファイルをコピー
COPY --from=builder /app/bot .

# 実行
CMD ["./bot"]
