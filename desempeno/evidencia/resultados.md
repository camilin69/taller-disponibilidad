# Resultados experimentales · tácticas de desempeño

_Generado el 2026-09-22 23:36:42_

## Tabla de métricas

| Corrida | Evidencia | Solicitudes | Latencia media | Latencia p95 | Throughput | Jitter | No procesadas | % desde caché |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| E0 | `/datos/e0_cliente.csv` | 128 | 1586.5 ms | 2799.0 ms | 1.96 req/s | 75.6 ms | 91 (71.1 %) | 0.0 % |
| E1 | `/datos/e1_cliente.csv` | 128 | 202.3 ms | 203.0 ms | 7.96 req/s | 1.3 ms | 0 (0.0 %) | 0.0 % |
| E2-run1 | `/datos/e2_run1_cliente.csv` | 128 | 16.8 ms | 201.0 ms | 8.07 req/s | 1.8 ms | 0 (0.0 %) | 92.2 % |
| E2-run2 | `/datos/e2_run2_cliente.csv` | 128 | 16.8 ms | 202.0 ms | 8.06 req/s | 1.8 ms | 0 (0.0 %) | 92.2 % |

## Efecto del caché (R2)

| Corrida | Desde caché | Calculadas | Latencia media desde caché | Latencia media calculada |
|---|---:|---:|---:|---:|
| E0 | 0 | 37 | n/a | 1586.5 ms |
| E1 | 0 | 128 | n/a | 202.3 ms |
| E2-run1 | 118 | 10 | 1.1 ms | 202.0 ms |
| E2-run2 | 118 | 10 | 1.1 ms | 202.2 ms |

## Detalle por corrida

### E0 (`/datos/e0_cliente.csv`)

- Ventana: 23:33:26.698 → 23:33:45.570 (18.9 s)
- Solicitudes: 128 enviadas, 37 exitosas, 91 no procesadas
- Latencia: media 1586.5 ms · p50 1588.0 ms · p95 2799.0 ms · min 228.0 ms · máx 2951.0 ms
- Throughput: 1.96 solicitudes exitosas por segundo
- Jitter: 75.6 ms de variación media entre solicitudes consecutivas
- Desde caché: 0 de 128 (0.0 %)
- Solicitudes no procesadas (91):
  - seq 38 a las 23:33:31.323: código=0 Get "http://servidor:8080/resumen/1007?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 39 a las 23:33:31.448: código=0 Get "http://servidor:8080/resumen/1008?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 40 a las 23:33:31.574: código=0 Get "http://servidor:8080/resumen/1009?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 41 a las 23:33:31.698: código=0 Get "http://servidor:8080/resumen/1000?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 42 a las 23:33:31.824: código=0 Get "http://servidor:8080/resumen/1001?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 43 a las 23:33:31.949: código=0 Get "http://servidor:8080/resumen/1002?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 44 a las 23:33:32.073: código=0 Get "http://servidor:8080/resumen/1003?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 45 a las 23:33:32.199: código=0 Get "http://servidor:8080/resumen/1004?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 46 a las 23:33:32.323: código=0 Get "http://servidor:8080/resumen/1005?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - seq 47 a las 23:33:32.448: código=0 Get "http://servidor:8080/resumen/1006?viajes=40": context deadline exceeded (Client.Timeout exceeded while awaiting headers)
  - ... y 81 más

### E1 (`/datos/e1_cliente.csv`)

- Ventana: 23:34:14.219 → 23:34:30.296 (16.1 s)
- Solicitudes: 128 enviadas, 128 exitosas, 0 no procesadas
- Latencia: media 202.3 ms · p50 202.0 ms · p95 203.0 ms · min 201.0 ms · máx 213.0 ms
- Throughput: 7.96 solicitudes exitosas por segundo
- Jitter: 1.3 ms de variación media entre solicitudes consecutivas
- Desde caché: 0 de 128 (0.0 %)
- Solicitudes no procesadas: 0

### E2-run1 (`/datos/e2_run1_cliente.csv`)

- Ventana: 23:34:52.269 → 23:35:08.135 (15.9 s)
- Solicitudes: 128 enviadas, 128 exitosas, 0 no procesadas
- Latencia: media 16.8 ms · p50 1.0 ms · p95 201.0 ms · min 0.0 ms · máx 203.0 ms
- Throughput: 8.07 solicitudes exitosas por segundo
- Jitter: 1.8 ms de variación media entre solicitudes consecutivas
- Desde caché: 118 de 128 (92.2 %)
- Solicitudes no procesadas: 0

### E2-run2 (`/datos/e2_run2_cliente.csv`)

- Ventana: 23:35:44.791 → 23:36:00.667 (15.9 s)
- Solicitudes: 128 enviadas, 128 exitosas, 0 no procesadas
- Latencia: media 16.8 ms · p50 1.0 ms · p95 202.0 ms · min 1.0 ms · máx 203.0 ms
- Throughput: 8.06 solicitudes exitosas por segundo
- Jitter: 1.8 ms de variación media entre solicitudes consecutivas
- Desde caché: 118 de 128 (92.2 %)
- Solicitudes no procesadas: 0

