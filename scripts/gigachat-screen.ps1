$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
$env:GIGACHAT_INTEGRATION = '1'
python scripts/gigachat-dev-sample.py
if ($LASTEXITCODE -ne 0) { throw 'DEV selection failed' }
go build -o tmp/semantic-eval.exe ./cmd/semantic-eval
if ($LASTEXITCODE -ne 0) { throw 'Evaluation build failed' }
foreach ($candidate in @('GigaChat-3-Ultra','GigaChat-2-Max','GigaChat-3-Pro','GigaChat-2')) {
    Write-Output ('DEV screening: ' + $candidate)
    & .\tmp\semantic-eval.exe -model $candidate -input artifacts/gigachat/dev-screening.jsonl -output ('artifacts/gigachat/dev-' + $candidate + '.json')
    if ($LASTEXITCODE -ne 0) { throw 'DEV screening failed' }
}
