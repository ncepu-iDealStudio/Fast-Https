# 设置开发环境变量
$env:FAST_HTTPS_ENV = "development"

# 清理旧的构建和进程
Write-Host "Cleaning up..."
if (Test-Path "fast-https.pid") {
    $pidJson = Get-Content "fast-https.pid" -Raw
    $pidObj = $pidJson | ConvertFrom-Json
    taskkill /F /PID $pidObj.pid 2>$null
    Remove-Item "fast-https.pid"
}

# 构建项目
Write-Host "Building project..."
go build -v

# 前台运行：本地调试时进程就在当前终端里
Write-Host "Starting in foreground..."
$env:FASTHTTPS_FOREGROUND = "1"
.\fast-https.exe start