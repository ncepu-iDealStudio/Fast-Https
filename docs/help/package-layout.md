# 安装后的目录布局

同一个二进制有两种布局，由编译期 build tag 决定。运行时不检测发行版，所以行为永远可预测。

## 对照表

| 内容 | 归档 / Docker / Windows（无 tag） | deb / rpm / apk（`-tags=rpm`） |
| --- | --- | --- |
| 主配置 | `./config/fast-https.json` | `/etc/fast-https/fast-https.json` |
| MIME 表 | `./config/mime.json` | `/etc/fast-https/mime.json` |
| 站点片段 | `./config/conf.d/` | `/etc/fast-https/conf.d/` |
| 服务器证书 | `./config/cert/` | `/etc/fast-https/cert/` |
| ACME 账户与缓存 | `./config/cert/acme/` | `/etc/fast-https/cert/acme/` |
| 站点根 | `./httpdoc/root/` | `/usr/share/fast-https/httpdoc/root/` |
| 日志 | `./logs/` | `/var/log/fast-https/` |
| pid | `./fast-https.pid` | `/run/fast-https/fast-https.pid` |
| 程序 | `fast-https` | `/usr/bin/fast-https` |
| systemd 单元 | — | `/usr/lib/systemd/system/fast-https.service` |
| 使用文档 | `docs/usage.html` | `/usr/share/doc/fast-https/usage.html` |
| man 页 | — | `/usr/share/man/man1/fast-https.1` |
| 日志轮转 | — | `/etc/logrotate.d/fast-https` |

## 为什么这样分

FHS 的核心是"按内容性质分区"，不是"按项目分区"：

- `/usr`：程序与只读数据，包管理器全权拥有，升级可整体替换
- `/etc`：运维改的配置，升级不覆盖（deb 的 conffile、rpm 的 `%config(noreplace)`）
- `/var/log`：程序自己写的可变状态，跨重启保留
- `/run`：tmpfs，重启清空，pid 与 socket 放这里

以前所有东西挤在 `/usr/share/fast-https/` 一个前缀下，导致日志写进只读目录——`ProtectSystem=strict` 或不可变发行版上直接起不来。

## 目录谁负责创建

| 路径 | 创建者 |
| --- | --- |
| `/etc/fast-https` 及配置、证书目录 | 包内 `type: dir` 声明 + `.keep` 占位 |
| `/etc/fast-https/cert` 权限 | postinstall `chmod 0700` |
| `/var/log/fast-https` | 包内 `type: dir`；运行时 `mkdirLogDir` 也会补建 |
| `/run/fast-https` | systemd `RuntimeDirectory=`；手工启动时 `writePidTo` 自建父目录 |

`/etc/logrotate.d/fast-https` 由包安装并登记为配置文件，轮转动作由 logrotate 自己的定时器（Debian/RHEL 的 `logrotate.timer` 或 cron）驱动，程序不参与。

`/run` 下的内容**不写进包**——rpm 不接受包内 `/run` 文件，且重启后会被清空。

## systemd 单元里的目录指令

```ini
ConfigurationDirectory=fast-https
LogsDirectory=fast-https
RuntimeDirectory=fast-https
```

systemd 236+ 在启动前按 `User=` 建好目录并设属主权限，停止时清理 `/run` 下的。这比在 postinstall 里 `mkdir` 更可靠，因为权限会跟着服务用户走。Alpine 没有 systemd，仍由 postinstall 兜底。

`WorkingDirectory=/usr/share/fast-https` 是有意保留的：只影响读取相对路径，不涉及写入，不违反 FHS。

## 配置里的相对路径按什么解析

路径按内容类型选基准（`config/paths.go`）：

| 配置字段 | 基准 |
| --- | --- |
| `ssl_certificate`、`ssl_certificate_key`、`http.include`、`acme.storage` | ETC 目录 |
| `location[].root`、`error_page`、`proxy_cache.path` | DATA 目录 |
| `log_root` | 工作目录（默认值来自 `DEFAULT_LOG_ROOT` 常量，通常不用写） |

1.5.0 的配置把路径写成 `./config/cert/x.pem`。拆掉安装根下的 `config` 层后，这些文本会多出一级前缀，`absPathIn` 会在目标不存在时自动剥掉 `config/` 再试一次，所以老配置不用改。

## 外部程序怎么获取路径

不要自己判断"是不是装成包了"。让主程序回答：

```bash
fast-https config path --json
```

```json
{
  "config": "/etc/fast-https/fast-https.json",
  "cert": "/etc/fast-https/cert",
  "log": "/var/log/fast-https",
  "pid": "/run/fast-https/fast-https.pid"
}
```

归档布局下同样可用，输出的是工作目录下的绝对路径。
