import type { SaludReplica } from '../tipos'

interface Props {
  replica: SaludReplica
  ganadas: number
  totalGanadas: number
}

/** Tarjeta con el estado en vivo de una replica (VIVA / CAIDA). */
export function TarjetaReplica({ replica, ganadas, totalGanadas }: Props) {
  const viva = replica.estado === 'VIVA'
  const porcentaje = totalGanadas > 0 ? (100 * ganadas) / totalGanadas : 0

  return (
    <article className={`tarjeta ${viva ? 'viva' : 'caida'}`}>
      <header>
        <h3>REPLICA {replica.replica_id}</h3>
        <span className={`insignia ${viva ? 'insignia-viva' : 'insignia-caida'}`}>
          {viva ? '● VIVA' : '● CAIDA'}
        </span>
      </header>

      <p className="url">{replica.url_base}</p>

      <dl>
        <div>
          <dt>Ultimo ping</dt>
          <dd>{viva ? `${replica.ultima_latencia_ms.toFixed(1)} ms` : 'sin respuesta'}</dd>
        </div>
        <div>
          <dt>Fallos consecutivos</dt>
          <dd>{replica.fallos_consecutivos}</dd>
        </div>
        <div>
          <dt>Sondeos</dt>
          <dd>
            {replica.total_sondeos} <span className="tenue">({replica.total_fallos} fallidos)</span>
          </dd>
        </div>
        <div>
          <dt>Ultimo cambio</dt>
          <dd>{replica.ultimo_cambio ? new Date(replica.ultimo_cambio).toLocaleTimeString() : '—'}</dd>
        </div>
      </dl>

      <div className="barra" title={`${ganadas} respuestas ganadas`}>
        <div className="barra-relleno" style={{ width: `${porcentaje}%` }} />
      </div>
      <p className="pie">
        gano {ganadas} carreras ({porcentaje.toFixed(1)}% de las respuestas)
      </p>
    </article>
  )
}
