#!/bin/sh
# 卸载前执行：先停服务，避免删掉二进制后进程仍在跑。
# 参数：deb 是 remove / purge / upgrade，rpm 是 0（卸载）或 1（升级），apk 无此参数。
# 注意：本文件必须保持 LF 行尾，CRLF 会让 shebang 变成 "#!/bin/sh\r"，
# dpkg 执行时报 "无法执行 prerm：没有那个文件或目录"，表现为卸载不掉。
set -e

BIN=/usr/bin/fast-https
# 独立 Monitor 的二进制也应该在卸载时被认出来：手工启动时它同样会脱离单元。
MON=/usr/bin/fast-https-monitor
PID_FILE=/run/fast-https/fast-https.pid
# 优雅停机上限：服务端收到 SIGTERM 会关闭监听并等在途请求（GracefulStopTimeout 5s）。
# 用 0.1s 的 tick 计数，避免依赖 sleep 的小数支持以外的工具。
STOP_TICKS=60

# is_our_process 判断这个 pid 是否确实在跑本程序。
#
# 只比对 pid 数字会误杀：进程早就退了，pid 可能已被别的程序复用。
# exe 带 " (deleted)" 说明二进制已经被删或被新包替换，这种情况也必须认——
# 正是「包删了、进程还在跑」的那个状态。
is_our_process() {
    _pid=$1
    _exe=$(readlink "/proc/$_pid/exe" 2>/dev/null) || _exe=""
    case "$_exe" in
        "$BIN" | "$BIN (deleted)" | "$MON" | "$MON (deleted)") return 0 ;;
    esac
    # /proc/PID/exe 读不到时（容器、权限受限）退回比对 argv[0]。
    _cmd=$(tr '\0' ' ' <"/proc/$_pid/cmdline" 2>/dev/null) || _cmd=""
    case "$_cmd" in
        "$BIN " | "$BIN" | "$MON " | "$MON") return 0 ;;
    esac
    return 1
}

# kill_pid 先发 SIGTERM 让进程优雅退出，超时再补 SIGKILL。
kill_pid() {
    _pid=$1
    case "$_pid" in '' | *[!0-9]*) return 0 ;; esac
    [ "$_pid" -gt 1 ] || return 0
    kill -0 "$_pid" 2>/dev/null || return 0
    is_our_process "$_pid" || return 0

    kill "$_pid" 2>/dev/null || true
    _t=0
    while [ "$_t" -lt "$STOP_TICKS" ]; do
        kill -0 "$_pid" 2>/dev/null || return 0
        sleep 0.1
        _t=$((_t + 1))
    done
    if kill -0 "$_pid" 2>/dev/null; then
        kill -9 "$_pid" 2>/dev/null || true
        echo "fast-https: pid $_pid ignored SIGTERM, sent SIGKILL" >&2
    fi
    return 0
}

# pid_from_file 从 pid 文件取 pid。文件内容是 JSON：{"pid":123}。
pid_from_file() {
    [ -r "$PID_FILE" ] || return 0
    sed -n 's/.*"pid"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$PID_FILE" 2>/dev/null
}

# stop_unit 停掉 systemd 单元并取消开机自启。
# 先停 Monitor 再停 Server：反过来的话 Monitor 会先看到一次「不可达」，
# 那是卸载造成的，不是要观测的故障。
stop_unit() {
    if command -v systemctl >/dev/null 2>&1; then
        systemctl stop fast-https-monitor >/dev/null 2>&1 || true
        systemctl disable fast-https-monitor >/dev/null 2>&1 || true
        systemctl stop fast-https >/dev/null 2>&1 || true
        systemctl disable fast-https >/dev/null 2>&1 || true
    fi
}

# kill_leftovers 杀掉单元之外的残留进程。
#
# 只做 stop_unit 是不够的：直接执行 `fast-https` 或 `fast-https start` 时，
# 进程会自己 daemonize（父进程退出、子进程 setsid 到 init 下面），
# 它不在单元的 cgroup 里，systemctl 碰不到它。包删掉后 /usr/bin/fast-https
# 成了 deleted inode，进程却还占着 80/443，重装后 start 只会报端口被占用，
# 而 /run 里的 pid 文件在 purge 时已经被删掉，事后无从查起。
# 所以这里两条路都走：pid 文件 + /proc 扫描。
kill_leftovers() {
    kill_pid "$(pid_from_file)"
    for _dir in /proc/[0-9]*; do
        kill_pid "${_dir#/proc/}"
    done
}

case "$1" in
    remove|purge|0)
        # 升级（deb 的 upgrade、rpm 的 $1=1）故意不走这里：升级时进程继续提供服务，
        # 新二进制靠 postinstall 处理，卸载才需要把它彻底停掉。
        stop_unit
        kill_leftovers
        ;;
    "")
        # apk 的 pre-deinstall 拿不到参数，无法区分升级与卸载：
        # 只停单元，不做强杀，避免升级时把手工启动的进程打断且无人拉起。
        stop_unit
        ;;
esac

exit 0
