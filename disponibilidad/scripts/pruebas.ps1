<#
.SYNOPSIS
  Ejecuta todas las pruebas automatizadas (unitarias + integracion).
.EXAMPLE
  .\scripts\pruebas.ps1
  .\scripts\pruebas.ps1 -Race
#>
param([switch]$Race)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

Write-Host "=== go vet ===" -ForegroundColor Cyan
go vet ./...

Write-Host "=== pruebas unitarias y de integracion ===" -ForegroundColor Cyan
if ($Race) {
  go test -race ./... -count=1
} else {
  go test ./... -count=1
}
