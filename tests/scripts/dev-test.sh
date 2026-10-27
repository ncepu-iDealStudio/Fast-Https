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

# 运行开发模式
echo "Starting in development mode..."
./fast-https dev