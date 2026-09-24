# Evidencia experimental

Archivos generados por las corridas reales del sistema. Se versionan porque son
la evidencia que exige el taller: *un número sin evidencia no cuenta como
medición*.

| Archivo | Origen | Contenido |
|---|---|---|
| `e0_cliente.csv` | `scripts/experimento.sh E0` | Línea base: W=1, sin caché |
| `e1_cliente.csv` | `scripts/experimento.sh E1` | Con concurrencia: W=8, sin caché |
| `e2_run1_cliente.csv` | `scripts/experimento.sh E2 run1` | Concurrencia + caché, primera corrida |
| `e2_run2_cliente.csv` | `scripts/experimento.sh E2 run2` | Concurrencia + caché, segunda corrida |
| `resultados.md` | `scripts/generar-metricas.sh` | Tabla de las cuatro métricas calculada a partir de los CSV |

## Columnas del CSV

```
secuencia, timestamp_envio, timestamp_respuesta, exito (0/1), desde_cache (0/1),
id_tarjeta, latencia_ms, codigo_http, segundo_relativo, experimento, error
```

Todos los timestamps están en UTC con milisegundos
(`2006-01-02T15:04:05.000Z`), de modo que los CSV de distintas corridas son
directamente comparables entre sí.

## Cómo se derivan las cuatro medidas

| Métrica | Cómo se calcula |
|---|---|
| Latencia (promedio y p95) | `timestamp_respuesta − timestamp_envio`, solo filas con `exito=1` |
| Throughput | Filas con `exito=1` dividido entre la duración total del experimento |
| Jitter | Diferencia absoluta entre la latencia de una solicitud y la anterior, promediada |
| Eventos no procesados | Filas con `exito=0`: no obtuvieron respuesta dentro de los 3 s de límite |
| % desde caché | Filas con `desde_cache=1` dividido entre el total |

Las filas con `exito=0` no entran en la latencia ni en el throughput: se cuentan
aparte, como indica el numeral 6 del enunciado.

## Resumen de las corridas incluidas

Ejecutadas el 22 de septiembre de 2026, 128 solicitudes cada una (8 req/s
durante 16 s) sobre 10 tarjetas.

| Corrida | Latencia media | p95 | Throughput | Jitter | No procesadas | % caché |
|---|---:|---:|---:|---:|---:|---:|
| E0 · `W=1`, sin caché | 1.586,5 ms | 2.799,0 ms | 1,96 req/s | 75,6 ms | 91 (71,1 %) | 0,0 % |
| E1 · `W=8`, sin caché | 202,3 ms | 203,0 ms | 7,96 req/s | 1,3 ms | 0 | 0,0 % |
| E2 · run1 | 16,8 ms | 201,0 ms | 8,07 req/s | 1,8 ms | 0 | 92,2 % |
| E2 · run2 | 16,8 ms | 202,0 ms | 8,06 req/s | 1,8 ms | 0 | 92,2 % |

## Nota sobre las 91 solicitudes no procesadas de E0

No son un error del experimento: son el problema que el taller pide reproducir.
En `e0_cliente.csv` las secuencias **1 a 37 tienen `exito=1`** y las **38 a 128
tienen `exito=0`**, todas con una latencia de exactamente 3.000 ms, que es el
tiempo límite del cliente.

La latencia crece de forma lineal, 75 ms por solicitud: es el déficit entre la
capacidad del servidor con `W=1` (5 req/s) y la ráfaga (8 req/s), es decir
`1/5 − 1/8` de segundo. Ese mismo número aparece como el jitter medido
(75,6 ms). A partir de la solicitud 38 la espera acumulada supera los 3 s y
ninguna alcanza a completarse.

Se conservan porque son la evidencia de la línea base: sin ellas no habría con
qué comparar E1 y E2.
