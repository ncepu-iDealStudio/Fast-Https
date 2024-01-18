# Fast-Https Web server

#### introduction
Fast-Https is a high-performance, multi-tasking web server product developed in Go language. It supports popular protocols and standards such as HTTP 1.1, HTTP 2.0, HTTPS, and RPC. It also offers reverse proxy functionality. Notably, the web server includes a built-in feature for generating self-signed SSL certificates, which greatly facilitates addressing the issue of server access based on the HTTPS protocol in testing environments.

Currently, we provide installation packages for both Windows and Linux platforms, with products for other platforms being gradually released.

#### Software Architecture
Fast-Https is designed and developed in a modular manner, where the core server module supports horizontal expansion and the addition of new functionalities through plugins. This enhances the capabilities of the server.

#### installation
1. You can obtain the corresponding version and installation package at https://gitee.com/ncepu-bj/fast-https/releases/tag；
2. Extract the corresponding installation package to the target directory on the server；
3. Modify the configuration file；

#### user guider
see there：https://idealstudio-ncepu.yuque.com/dkna2e/lbeklg?# 《Fast-Https产品说明》

#### 自行编译
1. Compile Windows status bar control program
   go build -ldflags "-s -w -H=windowsgui" -o monitor.exe monitor.go

2. Compile distribution package for the Linux platform.
   goreleaser release -f .goreleaser.yaml --snapshot --clean

   if you need,you can modify the config file:”goreleaser.yaml“

3. Compile distribution package for the Windows platform
   goreleaser release -f .goreleaser.windows.yaml --snapshot --clean


#### Contribution
1.Fork the repository

2.Create Feat_xxx branch

3.Commit your code

4.Create Pull Request
