<#
.SYNOPSIS
  Ejecuta el protocolo experimental completo del taller: E0, E1 y E2 (dos
  corridas de E2, como pide el enunciado) y genera la tabla de metricas.
.EXAMPLE
  .\scripts\todos-los-experimentos.ps1
#>
$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

& "$PSScriptRoot\experimento.ps1" -Experimento E0
& "$PSScriptRoot\experimento.ps1" -Experimento E1
# E2 se ejecuta dos veces para confirmar los valores (numeral 7 del enunciado).
& "$PSScriptRoot\experimento.ps1" -Experimento E2 -Corrida run1
& "$PSScriptRoot\experimento.ps1" -Experimento E2 -Corrida run2

& "$PSScriptRoot\generar-metricas.ps1"
