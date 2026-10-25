# 22. 配置文件自动重载（占位）

状态：📋 计划占位——暂缓（iteration-plan R7），当前为手工 `reload`/SIGINT

## 功能（目标）

监听 `config/fast-https.json` 与 include 目录变更，变更后自动执行与手工 reload 完全一致的校验-切换流程。

## 需求要点（拟定，待评审）

- 机制：`fsnotify` watch 主配置 + include 目录，800ms 防抖合并多次保存。
- 语义：复用 `config.Reload` 全链路（校验失败保留旧配置、快照原子切换），自动重载只是触发器，不引入新状态路径。
- 防御：编辑器原子写（rename）与截断写都要正确处理；校验失败记录日志并保留旧配置，不反复重试风暴。
- 开关：配置项（默认关闭），避免与外部编排工具（自己的 reload 逻辑）互相踩踏。

## 已具备的基础

- reload 全链路已快照化（[4-reload.md](4-reload.md)），触发器是唯一缺口。
- E2E：`FASTHTTPS_E2E_RELOAD=1` 可直接复用为自动重载的验收。

## 开放问题

- include 目录 watch 的递归粒度与句柄开销。
- 与 `reload` 命令并发触发时的去重。
