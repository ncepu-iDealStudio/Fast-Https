@echo off
cd /d E:\Open_source_project\fast-https
taskkill /F /IM go.exe /T >nul 2>&1
taskkill /F /IM gofmt.exe /T >nul 2>&1
for /f "tokens=2" %%a in ('tasklist /FI "IMAGENAME eq *.test.exe" /NH 2^>nul') do taskkill /F /PID %%a >nul 2>&1
echo KILL_DONE>> _gotest_run.log
go test -p 1 -count=1 -timeout 120s ./cmd/ ./config/ ./modules/core/ ./modules/core/request/ ./modules/core/events/ ./modules/core/response/ ./modules/core/server/ ./modules/static/ ./modules/proxy/ >> _gotest_run.log 2>&1
echo EXIT_CODE=%ERRORLEVEL%>> _gotest_run.log
