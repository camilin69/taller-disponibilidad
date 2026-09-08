<#
.SYNOPSIS
  Experimento E1 (caida abrupta): carga constante + inyeccion de falla.
.DESCRIPTION
  1. Lanza el cliente (carga constante durante DURACION_S segundos).
  2. En el segundo indicado por -SegundoInyeccion ejecuta el inyector de fallas,
     que registra su timestamp y mata el contenedor de la replica objetivo.
  3. Al terminar muestra el estado de las replicas y las ultimas lineas de la
     bitacora del monitor.
  La evidencia queda en evidencia/: CSV del cliente, monitor.log e inyector.log.
.EXAMPLE
  .\scripts\experimento-e1.ps1 -Replica B -Corrida run1
#>
param(
  [string]$Replica = "B",
  [string]$Corrida = "run1",
  [int]$Duracion = 40,
  [int]$Tasa = 20,
  [int]$SegundoInyeccion = 15
)

$ErrorActionPreference = "Stop"

# URL del dispatcher vista desde el host (configurable para evitar choques de puertos).
$urlDispatcher = if ($env:DISPATCHER_URL_PUBLICA) { $env:DISPATCHER_URL_PUBLICA } else { "http://localhost:8080" }
$raiz = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $raiz

$csv = "e1_${Corrida}_cliente.csv"
$contenedor = "recaudo-replica-" + $Replica.ToLower()

Write-Host "=== E1 ($Corrida): $Tasa req/s durante $Duracion s, se mata $contenedor en t+$SegundoInyeccion s ===" -ForegroundColor Cyan
Write-Host "Estado inicial:"
curl.exe -s "$urlDispatcher/estado"
Write-Host ""

# El cliente corre en segundo plano mientras el inyector actua.
$trabajo = Start-Job -ScriptBlock {
  param($dir, $csv, $duracion, $tasa)
  Set-Location $dir
  docker compose --profile carga run --rm `
    -e ETIQUETA=E1 `
    -e DURACION_S=$duracion `
    -e TASA_RPS=$tasa `
    -e CSV_SALIDA=/datos/$csv `
    cliente 2>&1
} -ArgumentList $raiz, $csv, $Duracion, $Tasa

Start-Sleep -Seconds $SegundoInyeccion

Write-Host "--- Inyectando la falla en $contenedor ---" -ForegroundColor Yellow
docker compose --profile caos run --rm inyector -objetivo $contenedor -modo docker -comando kill

# Se espera a que el monitor acumule los k fallos y registre la transicion.
Start-Sleep -Seconds 5
Write-Host ""
Write-Host "Estado tras la caida:" -ForegroundColor Yellow
curl.exe -s "$urlDispatcher/estado"
Write-Host ""

Write-Host "Esperando a que el cliente termine la carga..."
Wait-Job $trabajo | Out-Null
Receive-Job $trabajo
Remove-Job $trabajo

Write-Host ""
Write-Host "--- Bitacora del monitor ---" -ForegroundColor Cyan
Get-Content (Join-Path $raiz "evidencia\monitor.log") -Tail 10
Write-Host ""
Write-Host "--- Log del inyector ---" -ForegroundColor Cyan
Get-Content (Join-Path $raiz "evidencia\inyector.log") -Tail 4
Write-Host ""
Write-Host "Evidencia: evidencia/$csv, evidencia/monitor.log, evidencia/inyector.log" -ForegroundColor Green
Write-Host "Para volver a levantar la replica: .\scripts\recuperar-replica.ps1 -Replica $Replica"
