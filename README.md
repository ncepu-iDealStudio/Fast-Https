# Fast-Https Web服务器

#### 介绍
Fast-Https是一款基于Go语言开发的的多任务，高并发Web服务器产品，支持http1.1/http2.0、HTTPS、RPC等主流的协议和标准；能够实现发布配置普通Web应用以及配置反向代理的功能。特别的，Web服务器内置实现了自签名SSL证书的生成功能，能极大方便在测试环境下解决服务器基于https协议访问的问题；

目前，我们提供了Windows平台和Linux平台下的安装包，其它平台下的产品陆续推出中；


Fast-Https项目，已经加盟华为openeuler社区（https://gitee.com/src-openeuler/fast-https），欢迎更多的开发者参与进来，共同完善Fast-Https产品，让更多的用户能够使用到Fast-Https产品；


#### 软件架构
Fast-Https采用Go语言开发，基于Golang的net/http包实现，支持http1.1/http2.0、HTTPS、RPC等主流的协议和标准；


#### 安装教程

#### 软件架构
Fast-Https采用模块化的方式设计开发，核心服务器模块支持以插件方式横向扩展、添加新的功能，以增强服务器的功能；


#### 安装教程
1.  在https://gitee.com/ncepu-bj/fast-https/releases/ 获取相应的版本和安装包;
2.  将相应的安装包解压到服务器的目标目录下;
3.  修改配置文件;


#### 使用说明

见文档：https://idealstudio-ncepu.yuque.com/dkna2e/lbeklg?# 《Fast-Https产品说明》


#### 自行编译
1. 编译windows状态栏控制程序
    go build -ldflags "-s -w -H=windowsgui" -o monitor.exe monitor.go

2. 编译linux平台下的发行包
   goreleaser release -f .goreleaser.yaml --snapshot --clean
   如果需要，可以修改相应的编译配置文件”goreleaser.yaml“

3. 编译windows平台下的发行包
   goreleaser release -f .goreleaser.windows.yaml --snapshot --clean
    

#### 参与贡献

1.  Fork 本仓库
2.  新建 Feat_xxx 分支
3.  提交代码
4.  新建 Pull Request

