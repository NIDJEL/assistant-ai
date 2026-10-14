param([int]$Port = 8000)
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
if (-not (Test-Path -LiteralPath '.venv\Scripts\python.exe')) {
    python -m venv .venv
    if ($LASTEXITCODE -ne 0) { throw 'Не удалось создать окружение. Установите Python 3.12 или новее.' }
}
& '.\.venv\Scripts\python.exe' -m pip install -r requirements.lock
if ($LASTEXITCODE -ne 0) { throw 'Не удалось установить зависимости.' }
Write-Host "Ассистент Гена: http://localhost:$Port"
& '.\.venv\Scripts\python.exe' -m uvicorn app.main:app --host 127.0.0.1 --port $Port
