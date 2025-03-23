#!/bin/bash

# 设置开发环境变量
export FAST_HTTPS_ENV=development

# 清理旧的构建和进程
echo "Cleaning up..."
if [ -f "pid" ]; then
    pid=$(cat pid)
    kill -9 $pid 2>/dev/null
    rm pid
fi

# 构建项目
echo "Building project..."
go build -v

# 运行开发模式
echo "Starting in development mode..."
./fast-https start --dev 