<#
.SYNOPSIS
  Vuelve a levantar una replica caida para observar la transicion CAIDA -> VIVA.
.EXAMPLE
  .\scripts\recuperar-replica.ps1 -Replica B
#>
param([string]$Replica = "B")

$ErrorActionPreference = "Stop"

# URL del dispatcher vista desde el host (configurable para evitar choques de puertos).
$urlDispatcher = if ($env:DISPATCHER_URL_PUBLICA) { $env:DISPATCHER_URL_PUBLICA } else { "http://localhost:8080" }
$raiz = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $raiz

$contenedor = "recaudo-replica-" + $Replica.ToLower()
Write-Host "levantando $contenedor..."
docker start $contenedor | Out-Null
Start-Sleep -Seconds 3

Write-Host "estado del dispatcher:"
curl.exe -s "$urlDispatcher/estado"
Write-Host ""
Write-Host "bitacora:"
Get-Content (Join-Path $raiz "evidencia\monitor.log") -Tail 5
