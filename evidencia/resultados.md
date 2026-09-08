# Resultados experimentales

_Generado el 2026-09-08 04:23:34_

## Tabla comparativa

| Corrida | Solicitudes | Exitosas | Fallidas | % exito | Tiempo de deteccion | Fallos en ventana post-falla |
|---|---:|---:|---:|---:|---:|---:|
| E0 (`/datos/e0_cliente.csv`) | 800 | 800 | 0 | 100.00% | n/a (sin falla) | n/a |
| E1 (`/datos/e1_run1_cliente.csv`) | 800 | 800 | 0 | 100.00% | 2.280 s | 0 |
| E1 (`/datos/e1_run2_cliente.csv`) | 800 | 800 | 0 | 100.00% | 1.400 s | 0 |

## Detalle por corrida

### E0 (`/datos/e0_cliente.csv`)

- Ventana: 04:04:58.802 -> 04:05:38.761
- Latencia p50 = 11.00 ms, p95 = 21.00 ms
- Reparto de respuestas por replica: A=252 (31.5%), B=277 (34.6%), C=271 (33.9%)
- Solicitudes fallidas: 0

### E1 (`/datos/e1_run1_cliente.csv`)

- Ventana: 04:05:47.802 -> 04:06:27.758
- Latencia p50 = 11.00 ms, p95 = 21.00 ms
- Reparto de respuestas por replica: A=287 (35.9%), B=212 (26.5%), C=301 (37.6%)
- Solicitudes fallidas: 0

### E1 (`/datos/e1_run2_cliente.csv`)

- Ventana: 04:06:47.324 -> 04:07:27.279
- Latencia p50 = 12.00 ms, p95 = 23.00 ms
- Reparto de respuestas por replica: A=326 (40.8%), B=93 (11.6%), C=381 (47.6%)
- Solicitudes fallidas: 0

## Tiempos de deteccion (Ping/Echo)

| # | Inyeccion (t0) | Transicion en bitacora (t1) | Replica | Deteccion (t1-t0) |
|---|---|---|---|---:|
| 1 | 04:06:22.402 | 04:06:24.682 | B | **2.280 s** |
| 2 | 04:07:02.278 | 04:07:03.678 | B | **1.400 s** |
| 3 | 04:08:16.177 | 04:08:17.671 | B | **1.494 s** |

## Bitacora del monitor (transiciones)

```
[2026-09-08T04:06:24.682Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos de ping)
[2026-09-08T04:06:38.383Z] REPLICA_B CAIDA -> VIVA (2 exitos consecutivos de ping)
[2026-09-08T04:07:03.678Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos de ping)
[2026-09-08T04:07:37.375Z] REPLICA_B CAIDA -> VIVA (2 exitos consecutivos de ping)
[2026-09-08T04:08:17.671Z] REPLICA_B VIVA -> CAIDA (2 fallos consecutivos de ping)
[2026-09-08T04:08:34.369Z] REPLICA_B CAIDA -> VIVA (2 exitos consecutivos de ping)
```
