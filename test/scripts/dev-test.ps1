# 设置开发环境变量
$env:FAST_HTTPS_ENV = "development"

# 清理旧的构建和进程
Write-Host "Cleaning up..."
if (Test-Path "pid") {
    $pid = Get-Content "pid"
    taskkill /F /PID $pid 2>$null
    Remove-Item "pid"
}

# 构建项目
Write-Host "Building project..."
go build -v

# 运行开发模式
Write-Host "Starting in development mode..."
.\fast-https.exe start --dev 