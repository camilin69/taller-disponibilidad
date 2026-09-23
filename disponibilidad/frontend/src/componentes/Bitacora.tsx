interface Props {
  lineas: string[]
}

/** Bitacora del monitor: [timestamp] REPLICA_X VIVA -> CAIDA. */
export function Bitacora({ lineas }: Props) {
  return (
    <section className="bitacora">
      <h2>Bitacora del monitor</h2>
      {lineas.length === 0 ? (
        <p className="tenue">
          Sin cambios de estado todavia. Ejecute el inyector de fallas para provocar uno.
        </p>
      ) : (
        <pre>
          {lineas
            .slice()
            .reverse()
            .map((linea, i) => (
              <span key={i} className={linea.includes('-> CAIDA') ? 'linea-caida' : 'linea-viva'}>
                {linea}
                {'\n'}
              </span>
            ))}
        </pre>
      )}
    </section>
  )
}
