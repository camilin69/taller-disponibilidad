<#
.SYNOPSIS
  Utilidades compartidas por los scripts de PowerShell del taller.
#>

# Windows PowerShell 5.1 envuelve en un ErrorRecord cada linea que un
# ejecutable nativo escribe por stderr. Con $ErrorActionPreference = "Stop" eso
# aborta el script aunque el comando haya terminado bien, y Docker escribe todo
# el progreso de construccion por stderr.
#
# Por eso se invoca a docker con la preferencia relajada y se verifica el
# codigo de salida a mano, que es la senal fiable de si fallo o no.
#
# Es una funcion SIMPLE (sin bloque param) a proposito: con un parametro
# declarado, PowerShell intentaria interpretar los modificadores de docker como
# nombres de parametro de la funcion y se comeria banderas como "-d". Con
# $args los argumentos llegan verbatim.
function Invocar-Docker {
  $anterior = $ErrorActionPreference
  $ErrorActionPreference = "Continue"
  try {
    & docker @args
    if ($LASTEXITCODE -ne 0) {
      throw "docker $($args -join ' ') fallo con codigo $LASTEXITCODE"
    }
  } finally {
    $ErrorActionPreference = $anterior
  }
}

# Esperar-Servidor reintenta GET /salud hasta que el servidor responda.
function Esperar-Servidor {
  param(
    [string]$Url,
    [int]$IntentosMaximos = 60
  )

  for ($i = 0; $i -lt $IntentosMaximos; $i++) {
    try {
      $null = Invoke-RestMethod -Uri "$Url/salud" -TimeoutSec 2
      return
    } catch {
      Start-Sleep -Milliseconds 500
    }
  }
  throw "el servidor no respondio en $Url tras $IntentosMaximos intentos"
}

# Url-Servidor devuelve la URL publica del servidor vista desde el host.
function Url-Servidor {
  if ($env:SERVIDOR_URL_PUBLICA) { return $env:SERVIDOR_URL_PUBLICA }
  return "http://localhost:8090"
}
