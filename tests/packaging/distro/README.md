# 发行版实机验证

在真容器里装包、启动、轮转、卸载，验证 `build/goreleaser.yaml` 与 `build/packaging/` 的产物在真实发行版上确实能用。`tests/packaging/*_test.go` 守的是静态约定（行尾、清单、轮转配置），这里守的是"装上去到底跑不跑"。

## 覆盖范围

| 目标 | 镜像 | 验证内容 |
| --- | --- | --- |
| `deb` | `ubuntu:24.04` | FHS 文件树、conffile 登记、logrotate、启动监听 80/443、日志轮转、`remove` 保留、`purge` 清理 |
| `upgrade` | `ubuntu:24.04` | 旧版 `/usr/share/fast-https/config` 迁移到 `/etc/fast-https`、不覆盖现场文件、迁移后服务可用 |
| `rpm` | `almalinux:9` | 同上，另加重装时 `%config(noreplace)` 保留本地修改、`rpm -e` 清理 |
| `apk` | `alpine:3.21` | 无 systemd 时的目录兜底、logrotate 依赖、卸载保留数据（apk 没有 purge 语义） |

## 用法

```sh
# 先出包
goreleaser release -f build/goreleaser.yaml --snapshot --clean --skip=archive,publish,announce

# 全部跑一遍（deb → upgrade → rpm → apk）
sh tests/packaging/distro/run.sh all

# 只跑一个，指定架构
sh tests/packaging/distro/run.sh rpm arm64
```

`run.sh` 负责 `docker run --rm`、挂载 `dist/` 与脚本目录、传 `ARCH`。Git Bash / MSYS 下会关掉路径自动转换，避免 `/dist` 被改写成 Windows 路径。

## 约定

- **每个脚本必须 `source common.sh`**：里面注册了 `trap stop_service EXIT`。`fast-https start` 会留后台守护进程占着 80/443，不 stop 的话下一次测试直接 bind 失败，容器也退不干净。
- **脚本只读 `dist/`**：挂载是 `:ro`，验证不改产物。
- **判定输出两态**：`PASS` / `FAIL`，末尾 `=== RESULT: ALL-PASS | HAVE-FAILURES ===`，退出码即结果，可直接接 CI。
- **`INFO` 不算失败**：例如 rpm 卸载后被修改过的 `%config` 可能留 `.rpmsave`，属发行版正常行为，不判失败但要打出来。

`tests/packaging/distro_test.go` 守上面第一条与镜像/脚本映射，改脚本结构时它会先失败。

**跑之前先看 [docs/test/3-distro-verification.md](../../../docs/test/3-distro-verification.md)**：那里记录了上次实测的产物校验和、镜像摘要、逐项结论与已知环境差异。同一份产物不必重复跑，只在改了打包清单、维护脚本或运行期路径后才需要重跑。
