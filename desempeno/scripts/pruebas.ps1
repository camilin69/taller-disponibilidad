<#
.SYNOPSIS
  Ejecuta todas las pruebas automatizadas (unitarias + integracion).
.DESCRIPTION
  Si Go no esta instalado en el equipo, usa la imagen oficial de Go en Docker.
.EXAMPLE
  .\scripts\pruebas.ps1
  .\scripts\pruebas.ps1 -Race
#>
param([switch]$Race)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
. (Join-Path $PSScriptRoot "comun.ps1")

if (Get-Command go -ErrorAction SilentlyContinue) {
  Write-Host "=== go vet ===" -ForegroundColor Cyan
  go vet ./...
  Write-Host "=== pruebas unitarias y de integracion ===" -ForegroundColor Cyan
  if ($Race) { go test -race ./... -count=1 } else { go test ./... -count=1 }
} else {
  Write-Host "Go no esta instalado: se usa la imagen golang:1.21 en Docker" -ForegroundColor Yellow
  # El detector de carreras necesita cgo, y la imagen alpine no trae compilador.
  $orden = if ($Race) {
    "apk add --no-cache gcc musl-dev >/dev/null && go vet ./... && go test -race ./... -count=1"
  } else {
    "go vet ./... && go test ./... -count=1"
  }
  Invocar-Docker run --rm -v "${PWD}:/src" -w /src golang:1.21-alpine sh -c $orden
}
