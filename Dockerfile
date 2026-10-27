FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go env -w GOPROXY=https://goproxy.cn,direct

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o fast-https .

FROM alpine:latest

RUN apk --no-cache add ca-certificates \
 && mkdir -p /app/config/cert /app/config/conf.d /app/logs /app/httpdoc/root

WORKDIR /app

COPY --from=builder /app/fast-https .
COPY config/fast-https.json config/mime.json config/fastcgi.conf ./config/
COPY httpdoc/root/index.html httpdoc/root/favicon.ico ./httpdoc/root/

ENV FASTHTTPS_FOREGROUND=1

EXPOSE 8080 443

CMD ["./fast-https"]
