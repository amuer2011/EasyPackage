param([switch]$All, [string]$OutputRoot = 'dist/v1.0.0')
$ErrorActionPreference = 'Stop'
Set-Location (Split-Path $PSScriptRoot -Parent)
$oldOS = $env:GOOS; $oldArch = $env:GOARCH; $oldCgo = $env:CGO_ENABLED
try {
    $env:CGO_ENABLED = '0'
    $platforms = ,@('windows', 'amd64')
    if ($All) { $platforms = @(@('windows','amd64'), @('windows','arm64')) }
    foreach ($target in $platforms) {
        $env:GOOS = $target[0]; $env:GOARCH = $target[1]
        $dir = "$OutputRoot/$($target[0])-$($target[1])"
        New-Item -ItemType Directory -Force -Path $dir | Out-Null
        $name = 'EasyPackage'; $flags = '-s -w'
        if ($env:GOOS -eq 'windows') { $name += '.exe'; $flags += ' -H=windowsgui' }
        & go build -trimpath -ldflags $flags -o "$dir/$name" .
        if ($LASTEXITCODE -ne 0) { throw "Build failed: $dir" }
        Copy-Item -LiteralPath README.md -Destination "$dir/README.md"
        Write-Host "Ready: $dir/$name"
    }
} finally { $env:GOOS = $oldOS; $env:GOARCH = $oldArch; $env:CGO_ENABLED = $oldCgo }
