# 빌드 스테이지
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

# 바이너리 빌드
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# 실행 스테이지
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/main .

RUN mkdir -p /app/uploads && chmod 700 /app/uploads

EXPOSE 8080

CMD ["./main"]
