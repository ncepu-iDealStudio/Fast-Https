# Alpine 运行镜像：文件布局、路径常量与启动方式必须与 deb/rpm/apk 安装后一致。
#
# 关键点：二进制必须带 -tags=rpm 编译。否则 config/base_dir.go 会用归档布局
# （./config、./logs），而 -tags=rpm 才会把资源定位到 /etc/fast-https、
# /usr/share/fast-https、/var/log/fast-https 等 FHS 路径。
#
# 构建：docker build -f build/docker/alpine.Dockerfile -t fast-https:alpine .
# 运行：docker run --rm -p 80:80 -p 443:443 fast-https:alpine

# ---------------------------------------------------------------- 构建阶段
FROM golang:1.27-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go env -w GOPROXY=https://goproxy.cn,direct && go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -tags=rpm -trimpath -o /out/fast-https ./fast-https.go \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/fast-https-monitor ./monitor

# ---------------------------------------------------------------- 运行阶段
FROM alpine:3.21

# ca-certificates 与 logrotate 是 apk 包声明的硬依赖；容器里照样装上，
# 保证 /etc/logrotate.d/fast-https 与包安装效果一致。
RUN apk add --no-cache ca-certificates logrotate \
 && mkdir -p \
      /etc/fast-https/cert/acme \
      /etc/fast-https/conf.d \
      /usr/share/fast-https/httpdoc/root \
      /usr/share/doc/fast-https \
      /usr/share/man/man1 \
      /usr/share/man/en/man1 \
      /usr/share/licenses/fast-https \
      /usr/lib/systemd/system \
      /etc/logrotate.d \
      /var/log/fast-https \
      /run/fast-https \
      /var/lib/fast-https \
 && chmod 0700 /etc/fast-https/cert /etc/fast-https/cert/acme /var/lib/fast-https \
 && touch /var/log/fast-https/.keep /var/lib/fast-https/.keep

COPY --from=builder /out/fast-https         /usr/bin/fast-https
COPY --from=builder /out/fast-https-monitor /usr/bin/fast-https-monitor

# 与 goreleaser nfpms.contents 的目标路径逐条对齐。
COPY config/fast-https.json                 /etc/fast-https/fast-https.json
COPY config/mime.json                       /etc/fast-https/mime.json
COPY config/conf.d/default.json             /etc/fast-https/conf.d/default.json
COPY build/packaging/monitor.json           /etc/fast-https/monitor.json
COPY config/cert/.keep                      /etc/fast-https/cert/.keep
COPY config/cert/acme/.keep                 /etc/fast-https/cert/acme/.keep
COPY config/conf.d/.keep                    /etc/fast-https/conf.d/.keep
# .keep 占位由上面的 RUN touch 生成：logs/ 被 .dockerignore 排除，
# 不依赖 ignore 例外规则，避免不同 Docker 版本行为不一致。
COPY httpdoc/root/index.html                /usr/share/fast-https/httpdoc/root/index.html
COPY httpdoc/root/favicon.ico               /usr/share/fast-https/httpdoc/root/favicon.ico
COPY docs/usage.html                        /usr/share/doc/fast-https/usage.html
COPY build/packaging/man/fast-https.1       /usr/share/man/man1/fast-https.1
COPY build/packaging/man/fast-https.en.1    /usr/share/man/en/man1/fast-https.1
COPY LICENSE                                /usr/share/licenses/fast-https/LICENSE
COPY build/packaging/systemd/fast-https.service         /usr/lib/systemd/system/fast-https.service
COPY build/packaging/systemd/fast-https-monitor.service /usr/lib/systemd/system/fast-https-monitor.service
COPY build/packaging/logrotate/fast-https   /etc/logrotate.d/fast-https

# Alpine 没有 systemd，Monitor 靠外部编排按需启动；它的配置与 systemd 单元
# 用的是同一份 /etc/fast-https/monitor.json，事件落 /var/log/fast-https/monitor。
ENV FASTHTTPS_FOREGROUND=1
WORKDIR /usr/share/fast-https

EXPOSE 80 443

CMD ["fast-https", "start"]
