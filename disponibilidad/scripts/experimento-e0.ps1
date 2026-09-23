<#
.SYNOPSIS
  Experimento E0 (linea base): carga constante sin fallas.
.DESCRIPTION
  Genera TASA_RPS solicitudes por segundo durante DURACION_S segundos contra el
  dispatcher y deja el CSV en evidencia/e0_cliente.csv. Sirve para comprobar que
  la redundancia activa reparte las respuestas entre varias replicas.
.EXAMPLE
  .\scripts\experimento-e0.ps1
#>
param(
  [int]$Duracion = 40,
  [int]$Tasa = 20,
  [string]$Csv = "e0_cliente.csv"
)

$ErrorActionPreference = "Stop"

# URL del dispatcher vista desde el host (configurable para evitar choques de puertos).
$urlDispatcher = if ($env:DISPATCHER_URL_PUBLICA) { $env:DISPATCHER_URL_PUBLICA } else { "http://localhost:8080" }
Set-Location (Join-Path $PSScriptRoot "..")

Write-Host "=== E0 (linea base): $Tasa req/s durante $Duracion s ===" -ForegroundColor Cyan
Write-Host "Estado inicial de las replicas:"
curl.exe -s "$urlDispatcher/estado"
Write-Host ""

docker compose --profile carga run --rm `
  -e ETIQUETA=E0 `
  -e DURACION_S=$Duracion `
  -e TASA_RPS=$Tasa `
  -e CSV_SALIDA=/datos/$Csv `
  cliente

Write-Host ""
Write-Host "Estado final de las replicas:"
curl.exe -s "$urlDispatcher/estado"
Write-Host ""
Write-Host "CSV generado en evidencia/$Csv" -ForegroundColor Green
