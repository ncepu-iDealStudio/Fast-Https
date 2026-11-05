# 帮助文档

面向"把 fast-https 跑起来、装上去、出了问题要修"的场景。开发规范与流程在 [`../develop/`](../develop/README.md)，功能说明在 [`../feature/`](../feature/README.md)。

## 按场景入口

| 我想做什么 | 去哪 |
| --- | --- |
| 从源码构建、打 deb/rpm/apk、安装、卸载 | [build-and-install.md](build-and-install.md) |
| 搞清楚装完之后文件都在哪、为什么分两套布局 | [package-layout.md](package-layout.md) |
| 装包失败、卸载不掉、服务起不来 | [troubleshooting.md](troubleshooting.md) |
| 想知道别人踩过哪些坑（含已修复的历史问题） | [dev-issues.md](dev-issues.md) |

## 快速自检

装完之后按这三步确认没问题：

```bash
dpkg -L fast-https | grep -E '^/(etc|var|run)'   # 文件是否落在 FHS 路径
fast-https config path --json                     # 运行期四个绝对路径
sudo systemctl enable --now fast-https && systemctl status fast-https --no-pager
```

三条都正常，再去看 `sudo ss -ltnp | grep -E ':(80|443)'` 确认端口在听。

## 两套布局先记住这一句

同一个二进制，两种路径，靠编译期 build tag 区分，运行时不判断：

- **归档 / Docker / Windows**（解压即用）：全部相对工作目录，`./config`、`./logs`、`./httpdoc`
- **deb / rpm / apk**（系统安装）：按 FHS 分到 `/etc/fast-https`、`/usr/share/fast-https`、`/var/log/fast-https`、`/run/fast-https`

所以"配置文件在哪"这个问题，答案取决于怎么装的。外部程序不要自己猜，用 `fast-https config path --json` 问主程序。

## 维护约定

- 新增排障条目时，先加进 `troubleshooting.md`；确认是代码/打包缺陷并已修掉的，再在 `dev-issues.md` 记一条带根因的记录。
- 每条记录写清"症状 → 根因 → 修复 → 防回归"，防回归优先落成测试，而不是只写在文档里。
