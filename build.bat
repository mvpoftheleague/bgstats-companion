@echo off
setlocal

set BINARY=BgStatsCompanion.exe
set OUTPUT_DIR=dist

echo Building BgStats Companion (Go)...

:: Embed the Windows application manifest + app icon
echo Generating manifest resource...

:: Convert logo.png -> logo.ico using PowerShell System.Drawing
powershell -NoProfile -Command ^
  "Add-Type -AssemblyName System.Drawing; ^
   $img = [System.Drawing.Image]::FromFile((Resolve-Path 'assets\logo.png').Path); ^
   $bmp = New-Object System.Drawing.Bitmap($img, 32, 32); ^
   $ico = [System.Drawing.Icon]::FromHandle($bmp.GetHicon()); ^
   $fs  = New-Object System.IO.FileStream('assets\logo.ico',[System.IO.FileMode]::Create); ^
   $ico.Save($fs); $fs.Close(); $ico.Dispose(); $bmp.Dispose(); $img.Dispose()"

where rsrc >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo rsrc not found. Installing...
    go install github.com/akavel/rsrc@latest
)
rsrc -manifest app.manifest -ico assets\logo.ico -o rsrc.syso

:: Sync addon files from the addon directory (always use latest)
echo Syncing addon files...
copy /Y "..\addon\BgStats.lua" "assets\BgStats.lua" >nul
copy /Y "..\addon\BgStats.toc" "assets\BgStats.toc" >nul

:: Build the Windows executable (no console window)
go build -ldflags="-H=windowsgui -s -w" -o "%OUTPUT_DIR%\%BINARY%" .

if %ERRORLEVEL% neq 0 (
    echo Build failed!
    exit /b 1
)

echo.
echo Build successful: %OUTPUT_DIR%\%BINARY%
for %%A in ("%OUTPUT_DIR%\%BINARY%") do echo File size: %%~zA bytes

:: Deploy to AppData (kills running instance first)
set APPDATA_DIR=%APPDATA%\BgStats Companion
if exist "%APPDATA_DIR%\%BINARY%" (
    echo.
    echo Deploying to %APPDATA_DIR%...
    powershell -NoProfile -Command "Stop-Process -Name BgStatsCompanion -Force -ErrorAction SilentlyContinue; Start-Sleep 1; Copy-Item '%OUTPUT_DIR%\%BINARY%' '%APPDATA_DIR%\%BINARY%' -Force"
    echo Deployed.
)
