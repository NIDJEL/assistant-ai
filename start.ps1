param()
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
if (-not (Test-Path -LiteralPath '.env')) {
    Copy-Item -LiteralPath '.env.example' -Destination '.env'
    throw 'Создан .env. Заполните POSTGRES_PASSWORD, SESSION_SECRET и PUBLIC_URL, затем повторите запуск.'
}
docker compose up --build -d --wait
if ($LASTEXITCODE -ne 0) { throw 'Не удалось запустить приложение. Проверьте Docker и настройки .env.' }
Write-Host 'Ассистент Гена: http://localhost:8081'
