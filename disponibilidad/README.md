# RECAUDO-T · Tácticas de disponibilidad: Ping/Echo + Redundancia Activa

Implementación completa del taller de tácticas de disponibilidad para el servicio
de consulta de saldo **RECAUDO-T**: el sistema detecta automáticamente la caída
de una réplica (**Ping/Echo**, R1) y sigue atendiendo sin que el cliente lo note
(**Redundancia Activa**, R2).

| Escenario de calidad | Valor |
|---|---|
| Fuente del estímulo | Interna (un proceso servidor deja de responder) |
| Estímulo | Crash de una réplica |
| Respuesta | Se detecta la falla, se registra en bitácora y se sigue atendiendo con las réplicas restantes |
| Medida | Detección ≤ 3 s · el cliente no percibe errores |
| **Resultado medido** | **Detección 2,280 s / 1,400 s / 1,494 s · 100,00 % de éxito en E0 y E1** ([evidencia](evidencia/), [documentación](docs/)) |

Todo está escrito en **Go 1.21** (solo biblioteca estándar: `net/http`,
goroutines, `context`) siguiendo **Clean Architecture**, con un panel en
**React + TypeScript** y orquestación con **Docker Compose**.

---

## 1. Arranque en un solo comando

```bash
docker compose up --build
```

Eso levanta cinco procesos independientes:

| Servicio | Contenedor | Puerto (host) | Rol |
|---|---|---|---|
| `dispatcher` | `recaudo-dispatcher` | `8080` | Punto único de entrada + monitor Ping/Echo |
| `replica-a` | `recaudo-replica-a` | `8081` | Réplica A |
| `replica-b` | `recaudo-replica-b` | `8082` | Réplica B |
| `replica-c` | `recaudo-replica-c` | `8083` | Réplica C |
| `frontend` | `recaudo-frontend` | `3000` | Panel React (estado en vivo) |

Abra **<http://localhost:3000>** para ver el panel, o consulte la API directamente:

```bash
curl http://localhost:8080/estado
# {"A":"VIVA","B":"VIVA","C":"VIVA"}

curl http://localhost:8080/saldo/1234
# {"replica_id":"C","id_tarjeta":"1234","saldo":19500,"latencia_ms":7.4,...}
```

Para detener todo: `docker compose down`.

> **¿Puertos ocupados?** Todos los puertos publicados son variables de `.env`.
> Por ejemplo, para correr el sistema en 9080/9081-9083/9300:
>
> ```bash
> PUERTO_DISPATCHER=9080 PUERTO_REPLICA_A=9081 PUERTO_REPLICA_B=9082 > PUERTO_REPLICA_C=9083 PUERTO_FRONTEND=9300 > DISPATCHER_URL_PUBLICA=http://localhost:9080 docker compose up -d
> ```
>
> Los scripts de experimentos respetan `DISPATCHER_URL_PUBLICA`.

---

## 2. Ejecutar los experimentos

Los scripts existen en dos sabores equivalentes: `.ps1` (Windows/PowerShell) y
`.sh` (Linux/macOS/Git Bash).

### E0 — línea base (sin fallas)

```powershell
.\scripts\experimento-e0.ps1
```
```bash
./scripts/experimento-e0.sh
```

Genera 20 req/s durante 40 s y escribe `evidencia/e0_cliente.csv`. En la columna
`replica_id` debe verse la respuesta alternando entre A, B y C: eso evidencia
que la redundancia realmente está compitiendo.

### E1 — caída abrupta (dos corridas)

```powershell
.\scripts\experimento-e1.ps1 -Replica B -Corrida run1
.\scripts\recuperar-replica.ps1 -Replica B     # deja el sistema como estaba
.\scripts\experimento-e1.ps1 -Replica B -Corrida run2
.\scripts\recuperar-replica.ps1 -Replica B
```
```bash
./scripts/experimento-e1.sh B run1
./scripts/recuperar-replica.sh B
./scripts/experimento-e1.sh B run2
./scripts/recuperar-replica.sh B
```

El script lanza la carga, espera al segundo 15, ejecuta el inyector (que
**registra su timestamp antes** de hacer `docker kill`) y al final muestra la
bitácora. Evidencia producida:

- `evidencia/e1_run1_cliente.csv` — todas las solicitudes con `exito` 0/1.
- `evidencia/monitor.log` — `[timestamp] REPLICA_B VIVA -> CAIDA (...)`.
- `evidencia/inyector.log` — `[timestamp] INYECTOR objetivo=recaudo-replica-b ...`.

### Tabla de métricas

```powershell
.\scripts\generar-metricas.ps1
```
```bash
./scripts/generar-metricas.sh
```

Lee los tres archivos de evidencia y escribe `evidencia/resultados.md` con el
tiempo de detección (bitácora − inyector) y el % de solicitudes exitosas.

### Inyectar una falla manualmente

```powershell
.\scripts\inyectar-falla.ps1 -Replica B      # docker kill + timestamp
.\scripts\recuperar-replica.ps1 -Replica B   # docker start -> CAIDA -> VIVA
```

También se puede provocar el crash sin Docker, usando el gancho de caos de la
réplica: `curl -X POST http://localhost:8082/chaos/crash`.

### Ver el tráfico en los logs

Cada servicio deja una línea por consulta atendida, así que `docker compose logs`
muestra la redundancia activa en vivo:

```bash
docker compose logs -f dispatcher replica-a replica-b replica-c
```

```
recaudo-dispatcher | peticion GET /saldo/1234 -> 200 replica=A 8.0 ms 110 B cliente=172.19.0.1
recaudo-replica-a  | peticion GET /saldo/1234 -> 200 6.4 ms 53 B cliente=172.19.0.5
recaudo-replica-b  | peticion GET /saldo/1234 -> cancelada (descartada por la redundancia) 7.0 ms 0 B
recaudo-replica-c  | peticion GET /saldo/1234 -> cancelada (descartada por la redundancia) 7.4 ms 0 B
```

Una sola consulta llega a **las tres réplicas** (fan-out), gana la más rápida
(`replica=A` en el dispatcher) y las otras dos quedan canceladas: eso es
exactamente la táctica de redundancia activa.

| Variable | Defecto | Efecto |
|---|---|---|
| `LOG_PETICIONES` | `true` | Una línea por consulta de negocio (`/saldo`, `/chaos/crash`) |
| `LOG_VIGILANCIA` | `false` | Añade el tráfico periódico: `/ping`, `/salud` y el refresco del panel (`/estado`, `/estado/detalle`, `/bitacora`, `/metricas`) |
| `MONITOR_TRAZA` | `false` | En el dispatcher, imprime cada ping **enviado** por el monitor |

Para las corridas de carga conviene apagarlas (`LOG_PETICIONES=false`): a 20 req/s
son 800 líneas por corrida que estorban al leer la bitácora del monitor.

---

## 3. Ejecutar las pruebas

```powershell
.\scripts\pruebas.ps1          # go vet + unitarias + integración
.\scripts\pruebas.ps1 -Race    # además con detector de carreras
```
```bash
./scripts/pruebas.sh
./scripts/pruebas.sh -race
go test ./... -count=1                       # equivalente directo
go test ./tests/... -run TestE1 -v           # solo el escenario E1
```

| Paquete | Qué verifica |
|---|---|
| `internal/domain` | Entidades, formato de bitácora, validaciones |
| `internal/usecase` | Umbrales k/m, timeout de sondeo, carrera de redundancia, seguridad ante concurrencia |
| `internal/infra/config` | Lectura de T, t, k, m y lista de réplicas desde el entorno |
| `tests/integracion` | Sistema completo en memoria: E0, E1 (detección ≤ 3 s + 0 errores), recuperación, 503 sin réplicas, contrato de `/estado` |

Las pruebas de integración levantan réplicas HTTP reales y el dispatcher real
(mismo cableado que el binario de producción), matan una réplica en caliente y
verifican las dos tácticas. No requieren Docker.

---

## 4. API

### Dispatcher (`:8080`)

| Ruta | Descripción |
|---|---|
| `GET /saldo/{idTarjeta}` | Reenvía en paralelo a las réplicas VIVA y responde con la primera respuesta válida |
| `GET /estado` | `{"A":"VIVA","B":"CAIDA","C":"VIVA"}` (contrato del taller) |
| `GET /estado/detalle` | Estado enriquecido + configuración + métricas + bitácora (lo consume React) |
| `GET /bitacora?n=100` | Últimas líneas de la bitácora del monitor |
| `GET /metricas` | Contadores de la redundancia activa (total, éxitos, ganadoras) |
| `GET /salud` | Liveness del propio dispatcher |

### Réplica (`:8081`, `:8082`, `:8083`)

| Ruta | Descripción |
|---|---|
| `GET /ping` | Eco del Ping/Echo: `200 {"replica_id":"A"}` |
| `GET /saldo/{idTarjeta}` | `200 {"replica_id":"A","id_tarjeta":"1234","saldo":19500}` |
| `POST /chaos/crash` | Termina el proceso (solo lo usa el inyector) |
| `GET /salud` | Contadores de pings y consultas atendidas |

---

## 5. Configuración (variables de entorno)

Todos los parámetros viven en `.env`; **ninguno está fijo en el código**.

### Dispatcher

| Variable | Defecto | Significado |
|---|---|---|
| `PUERTO` | `8080` | Puerto de escucha |
| `REPLICAS` | `A=http://replica-a:8080,...` | Lista de réplicas. **Agregar una réplica es agregarla aquí**, no tocar el código |
| `MONITOR_T_MS` | `1000` | **T**: periodo entre sondeos |
| `MONITOR_TIMEOUT_MS` | `300` | **t**: espera máxima del eco (debe ser < T) |
| `MONITOR_K` | `2` | **k**: fallos consecutivos para marcar CAÍDA |
| `MONITOR_M` | `2` | **m**: éxitos consecutivos para volver a VIVA |
| `ESTADO_INICIAL` | `VIVA` | Estado con el que arrancan las réplicas |
| `CONSULTA_TIMEOUT_MS` | `1500` | Tiempo máximo de la carrera entre réplicas |
| `BITACORA_ARCHIVO` | `/datos/monitor.log` | Archivo de bitácora (volumen `./evidencia`) |
| `MONITOR_TRAZA` | `false` | `true` imprime cada ping enviado (depuración) |
| `LOG_PETICIONES` | `true` | Una línea por consulta atendida |
| `LOG_VIGILANCIA` | `false` | Incluye en esa traza el tráfico periódico (sondeos y panel) |

### Réplica

| Variable | Defecto | Significado |
|---|---|---|
| `REPLICA_ID` | — (obligatoria) | Identificador único (A, B, C…) |
| `PUERTO` | `8080` | Puerto interno |
| `LATENCIA_MIN_MS` / `LATENCIA_MAX_MS` | `2` / `25` | Latencia artificial de `/saldo`, para que ninguna réplica gane siempre |
| `SALDO_BASE` | `15000` | Base del saldo determinista por tarjeta |
| `LOG_PETICIONES` / `LOG_VIGILANCIA` | `true` / `false` | Traza de peticiones atendidas por la réplica |

### Cliente e inyector

| Variable | Defecto | Significado |
|---|---|---|
| `DISPATCHER_URL` | `http://localhost:8080` | Destino de la carga |
| `TASA_RPS` | `20` | Solicitudes por segundo |
| `DURACION_S` | `40` | Duración de la carga |
| `ID_TARJETA` | `1234` | Tarjeta consultada |
| `ETIQUETA` | `E0` | Experimento (queda en el CSV) |
| `CSV_SALIDA` | `/datos/cliente.csv` | CSV de evidencia |
| `OBJETIVO` | `recaudo-replica-b` | Contenedor a matar |
| `MODO_INYECTOR` | `docker` | `docker` (kill/stop) o `http` (`/chaos/crash`) |

---

## 6. Estructura del repositorio

```
.
├── cmd/                        # binarios (uno por proceso)
│   ├── dispatcher/             #   punto de entrada + monitor
│   ├── replica/                #   proceso servidor de saldo
│   ├── cliente/                #   generador de carga -> CSV
│   ├── inyector/               #   inyector de fallas (timestamp + docker kill)
│   └── metricas/               #   analiza la evidencia y arma la tabla
├── internal/
│   ├── domain/                 # ENTIDADES: Replica, Estado, Saldo, CambioEstado
│   ├── usecase/                # CASOS DE USO: Monitor (R1), ConsultarSaldo (R2), Registro
│   ├── adapter/                # ADAPTADORES
│   │   ├── httpin/             #   entrada HTTP (handlers dispatcher y réplica)
│   │   ├── gateway/            #   salida HTTP (ping y consulta a réplicas)
│   │   └── bitacora/           #   salida a archivo (monitor.log)
│   ├── infra/                  # INFRAESTRUCTURA: config, servidor, ensamblaje
│   └── metricas/               # análisis de CSV/bitácora/log del inyector
├── tests/integracion/          # pruebas de integración del sistema completo
├── frontend/                   # panel React + TypeScript (Vite, nginx)
├── deploy/                     # un Dockerfile por servicio
├── scripts/                    # experimentos E0/E1, inyección, métricas, pruebas
├── docs/                       # documentación (Markdown + DOCX/PDF)
├── evidencia/                  # CSV, monitor.log, inyector.log, resultados.md
├── docker-compose.yml
└── .env                        # todos los parámetros configurables
```

La documentación completa (diseño, capas, evidencia, tablas y respuestas a las
preguntas de análisis) está en [`docs/`](docs/).

---

## 7. Decisiones de diseño (resumen)

- **Una goroutine de sondeo por réplica.** Un ping colgado en A no retrasa el de
  B ni la atención de consultas; el timeout `t` se aplica con `context`.
- **Estado inicial VIVA.** El sistema queda operativo desde el primer segundo;
  si una réplica no arrancó todavía, el monitor la marca CAÍDA tras k fallos y
  la vuelve a marcar VIVA tras m ecos.
- **Sin health checks de Docker ni balanceadores.** La detección es
  exclusivamente el monitor propio, como exige la restricción del taller.
- **`restart: "no"` en las réplicas.** Si el inyector mata una réplica, debe
  quedarse caída hasta que se la levante a mano.
- **Latencia artificial aleatoria en `/saldo`.** Sin ella la réplica más rápida
  ganaría siempre y E0 no evidenciaría la competencia.
- **Respuesta válida = 200 + `saldo` ≥ 0 + `replica_id`.** Un 200 vacío no gana
  la carrera.
