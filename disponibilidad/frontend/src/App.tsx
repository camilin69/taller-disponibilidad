import { useCallback, useEffect, useRef, useState } from 'react'
import { URL_DISPATCHER, consultarSaldo, obtenerDetalle } from './api'
import { Bitacora } from './componentes/Bitacora'
import { PanelMetricas } from './componentes/PanelMetricas'
import { TarjetaReplica } from './componentes/TarjetaReplica'
import type { EstadoDetalle } from './tipos'

const PERIODO_REFRESCO_MS = 1000

export default function App() {
  const [detalle, setDetalle] = useState<EstadoDetalle | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [ultimaActualizacion, setUltimaActualizacion] = useState<Date | null>(null)
  const [pruebaSaldo, setPruebaSaldo] = useState<string>('')
  const enCurso = useRef(false)

  const refrescar = useCallback(async () => {
    if (enCurso.current) return // evita solapar peticiones si la red va lenta
    enCurso.current = true
    try {
      const datos = await obtenerDetalle()
      setDetalle(datos)
      setUltimaActualizacion(new Date())
      setError(null)
    } catch (e) {
      setError((e as Error).message)
    } finally {
      enCurso.current = false
    }
  }, [])

  // Sondeo periodico del endpoint /estado: el panel refleja en vivo lo que
  // decide el monitor Ping/Echo del dispatcher.
  useEffect(() => {
    void refrescar()
    const id = window.setInterval(() => void refrescar(), PERIODO_REFRESCO_MS)
    return () => window.clearInterval(id)
  }, [refrescar])

  const replicas = detalle?.replicas ?? []
  const vivas = replicas.filter((r) => r.estado === 'VIVA').length
  const totalGanadas = Object.values(detalle?.metricas.ganadoras ?? {}).reduce((a, b) => a + b, 0)

  const probarConsulta = async () => {
    setPruebaSaldo('consultando...')
    const res = await consultarSaldo('1234')
    setPruebaSaldo(`${res.ok ? 'OK' : 'ERROR'} — ${res.texto}`)
  }

  return (
    <div className="contenedor">
      <header className="cabecera">
        <div>
          <h1>RECAUDO-T · Panel de disponibilidad</h1>
          <p className="tenue">
            Ping/Echo + Redundancia Activa · dispatcher: <code>{URL_DISPATCHER}</code>
          </p>
        </div>
        <div className="estado-global">
          <span className={vivas === replicas.length ? 'insignia-viva' : 'insignia-caida'}>
            {vivas}/{replicas.length} replicas VIVA
          </span>
          <span className="tenue">
            {ultimaActualizacion
              ? `actualizado ${ultimaActualizacion.toLocaleTimeString()}`
              : 'conectando...'}
          </span>
        </div>
      </header>

      {error && (
        <div className="alerta">
          No se pudo consultar el dispatcher ({error}). ¿Esta corriendo <code>docker compose up</code>?
        </div>
      )}

      {detalle && <PanelMetricas config={detalle.config} metricas={detalle.metricas} />}

      <section className="rejilla">
        {replicas.map((r) => (
          <TarjetaReplica
            key={r.replica_id}
            replica={r}
            ganadas={detalle?.metricas.ganadoras[r.replica_id] ?? 0}
            totalGanadas={totalGanadas}
          />
        ))}
        {replicas.length === 0 && !error && <p className="tenue">Cargando estado de las replicas...</p>}
      </section>

      <section className="acciones">
        <button onClick={() => void probarConsulta()}>Consultar saldo de la tarjeta 1234</button>
        {pruebaSaldo && <span className="resultado-prueba">{pruebaSaldo}</span>}
      </section>

      <Bitacora lineas={detalle?.bitacora ?? []} />

      <footer className="pie-pagina">
        <p className="tenue">
          Para provocar una caida: <code>./scripts/inyectar-falla.sh B</code> (o{' '}
          <code>scripts\inyectar-falla.ps1 -Replica B</code> en Windows).
        </p>
      </footer>
    </div>
  )
}
