<#
.SYNOPSIS
  Inyector de fallas: registra el timestamp y mata la replica indicada.
.EXAMPLE
  .\scripts\inyectar-falla.ps1 -Replica B
#>
param(
  [string]$Replica = "B",
  [ValidateSet("kill", "stop")][string]$Comando = "kill"
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")

$contenedor = "recaudo-replica-" + $Replica.ToLower()
docker compose --profile caos run --rm inyector -objetivo $contenedor -modo docker -comando $Comando
