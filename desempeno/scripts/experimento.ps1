<#
.SYNOPSIS
  Ejecuta un experimento del taller de desempeno (E0, E1 o E2).
.DESCRIPTION
  Recrea el servidor con la configuracion del experimento (W y cache), espera a
  que responda, dispara la rafaga y deja el CSV en evidencia/.

  Recrear el servidor en cada corrida no es opcional: es lo que garantiza que
  el cache arranque vacio y que W sea el del experimento. Sin eso, E2 heredaria
  el cache de la corrida anterior y sus numeros no valdrian.
.EXAMPLE
  .\scripts\experimento.ps1 -Experimento E0
  .\scripts\experimento.ps1 -Experimento E2 -Corrida run2
#>
param(
  [Parameter(Mandatory = $true)]
  [ValidateSet("E0", "E1", "E2")]
  [string]$Experimento,

  [string]$Corrida = "",
  [int]$Duracion = 16,
  [int]$Tasa = 8,
  [int]$Tarjetas = 10
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..")
. (Join-Path $PSScriptRoot "comun.ps1")

# Configuracion de cada experimento segun el numeral 7 del enunciado.
$configuracion = @{
  "E0" = @{ Trabajadores = 1; Cache = "false"; Descripcion = "linea base: un solo trabajador, sin cache" }
  "E1" = @{ Trabajadores = 8; Cache = "false"; Descripcion = "con concurrencia: W=8, sin cache" }
  "E2" = @{ Trabajadores = 8; Cache = "true"; Descripcion = "con concurrencia y cache: W=8, cache activo" }
}[$Experimento]

$sufijo = if ($Corrida) { "_$Corrida" } else { "" }
$csv = "$($Experimento.ToLower())$($sufijo)_cliente.csv"
$etiqueta = if ($Corrida) { "$Experimento-$Corrida" } else { $Experimento }
$urlServidor = Url-Servidor

Write-Host "=== $Experimento ($($configuracion.Descripcion)) ===" -ForegroundColor Cyan
Write-Host "rafaga: $Tasa req/s durante $Duracion s = $($Tasa * $Duracion) solicitudes sobre $Tarjetas tarjetas"

# Recrear el servidor con la configuracion del experimento.
$env:TRABAJADORES = $configuracion.Trabajadores
$env:CACHE_ACTIVA = $configuracion.Cache
# Las trazas se apagan durante la carga: escribir una linea por solicitud
# mientras se mide la latencia contamina justo lo que se quiere medir.
$env:LOG_PETICIONES = "false"

Write-Host "recreando el servidor con W=$($configuracion.Trabajadores) cache=$($configuracion.Cache)..."
Invocar-Docker compose up -d --build --force-recreate servidor
Esperar-Servidor -Url $urlServidor

Write-Host "configuracion en vivo:"
curl.exe -s "$urlServidor/config"
Write-Host ""

# --build evita que compose intente PULL de una imagen que solo existe local
# ("pull access denied") antes de decidirse a construirla.
Invocar-Docker compose --profile carga run --rm --build `
  -e ETIQUETA=$etiqueta `
  -e DURACION_S=$Duracion `
  -e TASA_RPS=$Tasa `
  -e NUM_TARJETAS=$Tarjetas `
  -e CSV_SALIDA=/datos/$csv `
  cliente

Write-Host ""
Write-Host "configuracion final (trabajadores ocupados y entradas del cache):"
curl.exe -s "$urlServidor/config"
Write-Host ""
Write-Host "CSV generado en evidencia/$csv" -ForegroundColor Green
