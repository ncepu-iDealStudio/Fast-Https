# Fast-Https Web服务器

## 介绍

Fast-Https是一款基于Go语言开发的的多任务，高并发Web服务器产品，支持http1.1/http2.0、HTTPS、RPC等主流的协议和标准；能够实现发布配置普通Web应用以及配置反向代理的功能。特别的，Web服务器内置实现了自签名SSL证书的生成功能，能极大方便在测试环境下解决服务器基于https协议访问的问题；

目前，我们提供了Windows平台和Linux平台下的安装包，其它平台下的产品陆续推出中；

Fast-Https项目，已经加盟华为openeuler社区（<https://gitee.com/src-openeuler/fast-https），欢迎更多的开发者参与进来，共同完善Fast-Https产品，让更多的用户能够使用到Fast-Https产品；>

## 软件架构

Fast-Https采用Go语言开发，基于Golang的net/http包实现，支持http1.1/http2.0、HTTPS、RPC等主流的协议和标准；

Fast-Https采用模块化的方式设计开发，核心服务器模块支持以插件方式横向扩展、添加新的功能，以增强服务器的功能；

### 项目结构

```text
fast-https/
├── cmd/                           # 命令行相关代码
│   ├── commands.go                # 主要命令实现
│   ├── unix_linux_amd64.go        # Linux AMD64 平台特定代码
│   ├── reload_windows.go          # Windows 平台重载功能
│   ├── reload_other.go            # 其他平台重载功能
│   └── unix_other.go              # 其他 Unix 平台代码
│
├── config/                      # 配置相关
│   └── config.go                # 配置检查和管理
│
├── init/                       # 初始化相关
│   └── initialization.go       # 初始化实现
│
├── modules/                    # 核心功能模块
│   ├── core/                   # 核心功能
│   │   └── server/             # 服务器实现
│   └── httptohttps/            # HTTP 到 HTTPS 转换模块
│
├── output/                    # 输出相关
│   └── output.go              # 输出实现（包含 logo 等）
│
├── utils/                     # 工具类
│   ├── logger/                # 日志模块
│   │   └── logger.go          # 日志实现
│   └── ip/                    # IP 相关工具
│       └── ip.go              # IP 工具实现
│
├── go.mod                     # Go 模块定义
├── go.sum                     # Go 依赖版本锁定
├── fast-https.go             # 主程序入口
├── monitor.go                # 监控程序（Windows状态栏）
├── .goreleaser.yaml          # Linux 发布配置
├── .goreleaser.windows.yaml  # Windows 发布配置
└── README.md                 # 项目说明文档
```

### 服务启动流程

服务启动过程按照以下步骤进行：

1. 程序入口（fast-https.go）
   - 初始化日志系统
   - 通过 cmd.RootCmd() 执行根命令

2. 命令处理（cmd/commands.go）
   - 解析命令行参数
   - 处理不同的命令：启动、停止、重载等
   - 对于"启动"命令：
     - 执行预检查（端口可用性）
     - 初始化服务器组件

3. 服务器初始化（modules/core/server/server.go）
   - 创建新的服务器实例
   - 设置信号处理器（SIGTERM、SIGINT、SIGQUIT）
   - 根据配置初始化监听器
   - 注册核心模块

4. 监听器设置
   - 配置 HTTP/HTTPS 端口
   - 初始化连接处理器
   - 支持 HTTP/1.1 和 HTTP/2

5. 模块初始化
   - 初始化安全模块
   - 设置动态日志
   - 准备其他核心功能

6. 服务运行
   - 为每个监听器启动协程
   - 处理传入连接
   - 管理优雅关闭和重载

信号处理说明：

- SIGTERM：优雅关闭
- SIGINT（Ctrl+C）：
  - 前台模式：停止服务
  - 守护进程模式：重载配置
- SIGQUIT：停止服务

## 安装教程

1. 在 <https://gitee.com/ncepu-bj/fast-https/releases/> 获取相应的版本和安装包
2. 将相应的安装包解压到服务器的目标目录下
3. 修改配置文件

## 使用说明

见文档：<https://idealstudio-ncepu.yuque.com/dkna2e/lbeklg?#> 《Fast-Https产品说明》

## 自行编译

1. 编译windows状态栏控制程序
    go build -ldflags "-s -w -H=windowsgui" -o monitor.exe monitor.go

2. 编译linux平台下的发行包
   goreleaser release -f .goreleaser.yaml --snapshot --clean
   如果需要，可以修改相应的编译配置文件"goreleaser.yaml"

3. 编译windows平台下的发行包
   goreleaser release -f .goreleaser.windows.yaml --snapshot --clean

## 参与贡献

1. Fork 本仓库
2. 新建 Feat_xxx 分支
3. 提交代码
4. 新建 Pull Request
