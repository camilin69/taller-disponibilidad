import type { ConfigMonitor, Metricas } from '../tipos'

interface Props {
  config: ConfigMonitor
  metricas: Metricas
}

/** Resumen de la configuracion del monitor y de las metricas acumuladas. */
export function PanelMetricas({ config, metricas }: Props) {
  const porcentaje = metricas.total > 0 ? (100 * metricas.exitosas) / metricas.total : 100

  return (
    <section className="panel">
      <div className="panel-bloque">
        <h2>Monitor Ping/Echo (R1)</h2>
        <ul className="lista-config">
          <li>
            <span>T (periodo de sondeo)</span>
            <strong>{config.T_intervalo_ms} ms</strong>
          </li>
          <li>
            <span>t (timeout del eco)</span>
            <strong>{config.t_timeout_ms} ms</strong>
          </li>
          <li>
            <span>k (fallos para CAIDA)</span>
            <strong>{config.k_fallos_para_caida}</strong>
          </li>
          <li>
            <span>m (exitos para VIVA)</span>
            <strong>{config.m_exitos_para_viva}</strong>
          </li>
          <li>
            <span>Deteccion esperada (k·T)</span>
            <strong>{(config.deteccion_esperada_ms / 1000).toFixed(1)} s</strong>
          </li>
        </ul>
      </div>

      <div className="panel-bloque">
        <h2>Redundancia activa (R2)</h2>
        <div className="metricas">
          <div className="metrica">
            <span className="metrica-valor">{metricas.total}</span>
            <span className="metrica-etiqueta">consultas</span>
          </div>
          <div className="metrica">
            <span className="metrica-valor ok">{metricas.exitosas}</span>
            <span className="metrica-etiqueta">exitosas</span>
          </div>
          <div className="metrica">
            <span className={`metrica-valor ${metricas.fallidas > 0 ? 'error' : ''}`}>
              {metricas.fallidas}
            </span>
            <span className="metrica-etiqueta">fallidas</span>
          </div>
          <div className="metrica">
            <span className="metrica-valor">{porcentaje.toFixed(2)}%</span>
            <span className="metrica-etiqueta">% de exito</span>
          </div>
        </div>
      </div>
    </section>
  )
}
