$ErrorActionPreference = 'Stop'
$launcher = Join-Path $PSScriptRoot 'service.py'
if (Get-Command python -ErrorAction SilentlyContinue) {
    & python $launcher up @args
} elseif (Get-Command py -ErrorAction SilentlyContinue) {
    & py -3 $launcher up @args
} else {
    throw 'Install Python 3.10+ and Docker Desktop, then run this command again.'
}
exit $LASTEXITCODE
