<#
.SYNOPSIS
  Calcula la tabla de metricas del taller (latencia, throughput, jitter,
  eventos no procesados y % desde cache) a partir de los CSV recolectados.
.EXAMPLE
  .\scripts\generar-metricas.ps1
#>
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
. (Join-Path $PSScriptRoot "comun.ps1")

Invocar-Docker compose --profile reporte run --rm --build metricas `
  -csv "/datos/e0_cliente.csv,/datos/e1_cliente.csv,/datos/e2_run1_cliente.csv,/datos/e2_run2_cliente.csv" `
  -salida /datos/resultados.md

Write-Host "reporte generado en evidencia/resultados.md" -ForegroundColor Green
