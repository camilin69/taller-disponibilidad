import type { Estado, EstadoDetalle } from './tipos'

// La URL del dispatcher se inyecta en tiempo de arranque del contenedor
// (public/config.js), no se compila dentro del bundle.
declare global {
  interface Window {
    __CONFIG__?: { DISPATCHER_URL?: string }
  }
}

export const URL_DISPATCHER: string =
  window.__CONFIG__?.DISPATCHER_URL?.replace(/\/$/, '') || 'http://localhost:8080'

/** GET /estado/detalle: estado enriquecido para el panel. */
export async function obtenerDetalle(senal?: AbortSignal): Promise<EstadoDetalle> {
  const respuesta = await fetch(`${URL_DISPATCHER}/estado/detalle?bitacora=30`, { signal: senal })
  if (!respuesta.ok) throw new Error(`El dispatcher respondio ${respuesta.status}`)
  return (await respuesta.json()) as EstadoDetalle
}

/** GET /estado: contrato simple {"A":"VIVA","B":"CAIDA"} exigido por el taller. */
export async function obtenerEstado(senal?: AbortSignal): Promise<Record<string, Estado>> {
  const respuesta = await fetch(`${URL_DISPATCHER}/estado`, { signal: senal })
  if (!respuesta.ok) throw new Error(`El dispatcher respondio ${respuesta.status}`)
  return (await respuesta.json()) as Record<string, Estado>
}

/** GET /saldo/{id}: consulta manual desde el panel (prueba de humo). */
export async function consultarSaldo(idTarjeta: string): Promise<{
  ok: boolean
  texto: string
}> {
  try {
    const respuesta = await fetch(`${URL_DISPATCHER}/saldo/${encodeURIComponent(idTarjeta)}`)
    const cuerpo = await respuesta.json()
    if (!respuesta.ok) {
      return { ok: false, texto: `HTTP ${respuesta.status}: ${cuerpo.error ?? 'error'}` }
    }
    return {
      ok: true,
      texto: `replica ${cuerpo.replica_id} respondio saldo ${cuerpo.saldo} en ${cuerpo.latencia_ms} ms`,
    }
  } catch (error) {
    return { ok: false, texto: `sin respuesta: ${(error as Error).message}` }
  }
}
