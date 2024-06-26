FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go env -w GOPROXY=https://goproxy.cn,direct

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo .

FROM alpine:latest  

RUN apk --no-cache add ca-certificates

WORKDIR /app

COPY --from=builder /app/fast-https .

COPY config/ ./config/
COPY httpdoc/ ./httpdoc/
COPY logs/ ./logs/

EXPOSE 8080 443

# 运行服务器
CMD ["./fast-https"]
