#!/bin/bash

# 设置开发环境变量
export FAST_HTTPS_ENV=development

# 清理旧的构建和进程
echo "Cleaning up..."
if [ -f "fast-https.pid" ]; then
    pid=$(cat fast-https.pid | sed -E 's/[^0-9]//g')
    kill -9 $pid 2>/dev/null
    rm fast-https.pid
fi

# 构建项目
echo "Building project..."
go build -v

# 前台运行：本地调试时进程就在当前终端里，Ctrl+C 之后直接收尾
echo "Starting in foreground..."
FASTHTTPS_FOREGROUND=1 ./fast-https start