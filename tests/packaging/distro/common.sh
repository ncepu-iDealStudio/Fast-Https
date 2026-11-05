# shellcheck shell=sh
# 容器内实机验证的公共函数，由 deb.sh / rpm.sh / apk.sh / upgrade.sh source。
# 只依赖 POSIX sh：Alpine 的 busybox ash 也要能跑。

fail=0
pass() { echo "PASS  $1"; }
bad()  { echo "FAIL  $1"; fail=1; }

expect_present() { if [ -e "$1" ]; then pass "$2"; else bad "$2（$1 不存在）"; fi; }
expect_absent()  { if [ -e "$1" ]; then bad "$2（$1 仍存在）"; else pass "$2"; fi; }

# 容器里 runInForeground() 判定为真，start 不会 daemonize，会一直占着前台。
# 所以必须自己转后台，再等端口真的在听；日志重定向到文件，失败时能贴出来。
start_server() {
    fast-https start >/tmp/fast-https-start.log 2>&1 &
    i=0
    while [ "$i" -lt 60 ]; do
        if listen_https; then return 0; fi
        i=$((i + 1))
        sleep 0.5
    done
    echo "--- start 输出 ---"
    tail -20 /tmp/fast-https-start.log
    return 1
}

# 端口探测不用 ss：Alpine 基础镜像没有 iproute2，curl 有。
listen_http()  { curl -s  --max-time 2 -o /dev/null "http://127.0.0.1/"  2>/dev/null; }
listen_https() { curl -sk --max-time 2 -o /dev/null "https://127.0.0.1/" 2>/dev/null; }

# 容器是阻塞的：start 留下的后台进程占着 80/443，不 stop 的话脚本退出后
# 容器还在，下一次测试直接 bind 失败。卸载后二进制可能已删，失败一律忽略。
stop_service() { fast-https stop >/dev/null 2>&1 || true; }
trap stop_service EXIT INT TERM

# 打一次请求，产生访问日志。
hit() { curl -sk --max-time 5 -o /dev/null "https://127.0.0.1/" 2>/dev/null || true; }

report() {
    echo "=== RESULT: $([ "$fail" -eq 0 ] && echo ALL-PASS || echo HAVE-FAILURES) ==="
    exit "$fail"
}
