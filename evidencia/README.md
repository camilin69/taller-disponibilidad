# Evidencia experimental

Archivos generados por las corridas reales del sistema (no son ejemplos).

| Archivo | Origen | Contenido |
|---|---|---|
| `e0_cliente.csv` | `scripts/experimento-e0.sh` | 800 solicitudes de la línea base E0 (sin fallas) |
| `e1_run1_cliente.csv` | `scripts/experimento-e1.sh B run1` | 800 solicitudes de la primera corrida de E1 |
| `e1_run2_cliente.csv` | `scripts/experimento-e1.sh B run2` | 800 solicitudes de la segunda corrida de E1 |
| `monitor.log` | Dispatcher (volumen `/datos`) | Bitácora de cambios de estado del monitor Ping/Echo |
| `inyector.log` | Inyector de fallas | Timestamp registrado justo antes de matar cada réplica |
| `resultados.md` | `scripts/generar-metricas.sh` | Tabla de métricas calculada a partir de los archivos anteriores |

Columnas del CSV del cliente:

`secuencia, timestamp_envio, timestamp_respuesta, exito (0/1), replica_id,
latencia_ms, codigo_http, segundo_relativo, experimento, error`

Todos los timestamps están en UTC con milisegundos
(`2006-01-02T15:04:05.000Z`), de modo que el CSV, la bitácora y el log del
inyector son directamente comparables entre sí.

Resumen de las corridas incluidas:

| Corrida | Solicitudes | % de éxito | Tiempo de detección |
|---|---:|---:|---:|
| E0 | 800 | 100,00 % | n/a (sin falla) |
| E1 · run1 | 800 | 100,00 % | 2,280 s |
| E1 · run2 | 800 | 100,00 % | 1,400 s |
| Verificación manual del panel (sin carga) | — | — | 1,494 s |
