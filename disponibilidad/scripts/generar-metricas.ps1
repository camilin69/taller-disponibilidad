<#
.SYNOPSIS
  Calcula la tabla de metricas del taller a partir de la evidencia recolectada
  (CSV del cliente + bitacora del monitor + log del inyector).
.EXAMPLE
  .\scripts\generar-metricas.ps1
#>
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

docker compose --profile reporte run --rm metricas `
  -csv "/datos/e0_cliente.csv,/datos/e1_run1_cliente.csv,/datos/e1_run2_cliente.csv" `
  -bitacora /datos/monitor.log `
  -inyector /datos/inyector.log `
  -salida /datos/resultados.md

Write-Host "reporte generado en evidencia/resultados.md" -ForegroundColor Green
