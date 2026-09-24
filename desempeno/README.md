# RECAUDO-T · Tácticas de desempeño: Introducir Concurrencia + Caché

Implementación del taller de tácticas de desempeño para el **resumen mensual de
viajes** de RECAUDO-T: el servicio atiende una ráfaga de solicitudes
concurrentes sin descartarlas (**Introducir Concurrencia**, R1) y responde de
inmediato cuando el resumen ya se calculó (**Mantener Múltiples Copias de
Datos**, R2).

| Escenario de calidad | Valor |
|---|---|
| Fuente del estímulo | Usuarios (muchas solicitudes simultáneas desde la app móvil) |
| Estímulo | Ráfaga de solicitudes concurrentes al resumen mensual (128 en 16 s) |
| Respuesta | El sistema sigue respondiendo en un tiempo aceptable y no descarta solicitudes |
| Medida | Latencia p95 dentro de un límite razonable · throughput sostenido · sin solicitudes descartadas |
| **Resultado medido** | **Throughput ×4,06 · latencia media de 1.586 ms a 16,8 ms · de 91 solicitudes descartadas a 0 · 92,2 % servido desde caché** ([evidencia](evidencia/), [documentación](docs/documentacion.md)) |

Está escrito en **Go 1.21** (solo biblioteca estándar: `net/http`, goroutines,
canales, `context`, `sync`) siguiendo **Clean Architecture**, con orquestación
en **Docker Compose**.

> Este taller es independiente del [taller de disponibilidad](../README.md), que
> vive en la raíz del repositorio. Son dos módulos de Go y dos stacks de Compose
> separados: se pueden levantar a la vez sin chocar (disponibilidad publica en
> 8080, este en 8090).

---

## 1. Arranque en un solo comando

```bash
docker compose up --build
```

Eso levanta un solo proceso:

| Servicio | Contenedor | Puerto (host) | Rol |
|---|---|---|---|
| `servidor` | `recaudo-servidor` | `8090` | Resumen mensual + pool de trabajadores + caché |

Consulte la API:

```bash
curl "http://localhost:8090/resumen/1234?viajes=40"
# {"id_tarjeta":"1234","total_viajes":40,"gasto_total":151625,"desde_cache":false}

curl http://localhost:8090/config
# {"trabajadores":1,"trabajadores_ocupados":0,"cola_max":256,...,"cache_activa":false,...}
```

Para detener todo: `docker compose down`.

> **¿Puerto ocupado?** Está en `.env`:
>
> ```bash
> PUERTO_SERVIDOR=9090 SERVIDOR_URL_PUBLICA=http://localhost:9090 docker compose up -d
> ```
>
> Los scripts de experimentos respetan `SERVIDOR_URL_PUBLICA`.

---

## 2. Contrato de la API

| Ruta | Uso |
|---|---|
| `GET /resumen/{idTarjeta}?viajes=N` | Calcula (o recupera del caché) el resumen de los últimos N viajes. `200` con `{"id_tarjeta","total_viajes","gasto_total","desde_cache"}`. `503` si la cola está llena. |
| `GET /config` | Cuántos trabajadores están activos y cuántas entradas tiene el caché **en ese momento**: es la verificación en vivo de que R1 y R2 funcionan. |
| `GET /salud` | Contadores básicos del proceso (lo usa el cliente para esperar el arranque). |

La respuesta trae además la cabecera `X-Desde-Cache: true|false`, que es lo que
permite ver la táctica en vivo en los logs sin leer el cuerpo.

---

## 3. Cómo funcionan las dos tácticas

### R1 · Introducir Concurrencia

El cálculo costoso **solo ocurre dentro del pool** de `W` trabajadores
([`internal/usecase/pool.go`](internal/usecase/pool.go)). No se delega en el
servidor HTTP: `net/http` arranca una goroutine por petición, así que calcular
dentro del handler haría imposible medir `W=1`. El pool es un conjunto fijo de
`W` goroutines que consumen de un canal acotado (`COLA_MAX`):

- Si hay un trabajador libre, el cálculo empieza de inmediato.
- Si no, la solicitud espera su turno en la cola.
- Si la cola está llena, se responde `503` sin esperar. Es un **evento no
  procesado**, la cuarta medida de la teoría.
- Si el cliente se rinde mientras la solicitud está en cola, el trabajador la
  descarta en vez de gastar su turno en un cálculo que nadie leerá.

### R2 · Mantener Múltiples Copias de Datos (caché)

El caché se consulta **antes** de encolar
([`internal/usecase/obtener_resumen.go`](internal/usecase/obtener_resumen.go)).
Ese orden es la razón de ser de la táctica: si el caché se consultara dentro del
trabajador, un acierto seguiría haciendo fila detrás de los cálculos y dejaría
de ser instantáneo.

La clave es `(id_tarjeta, viajes)`, no solo la tarjeta: el resumen de los
últimos 40 viajes no es el mismo que el de los últimos 60.

### El cálculo costoso

[`internal/adapter/calculo/viajes.go`](internal/adapter/calculo/viajes.go) hace
dos cosas:

1. **Trabajo real**: recorre los viajes del mes uno por uno y acumula el gasto
   (`domain.SumarViajes`). Es determinista, así que una respuesta del caché es
   indistinguible de una recalculada.
2. **Costo de lectura**: recorrer el historial de un mes real significa leer
   muchos registros de la base de datos, y eso es espera de E/S. Se modela
   rellenando hasta completar `COSTO_CALCULO_MS` (200 ms por defecto).

Al rellenar hasta un total fijo, todo cálculo tarda lo mismo
(**restricción 1** del enunciado), de modo que las diferencias entre E0, E1 y E2
vienen de las tácticas y no del ruido.

---

## 4. Ejecutar los experimentos

Los scripts existen en dos sabores equivalentes: `.ps1` (Windows/PowerShell) y
`.sh` (Linux/macOS/Git Bash). Cada uno **recrea el servidor** con la
configuración del experimento, espera a que responda y dispara la ráfaga.

Recrear no es opcional: es lo que garantiza que el caché arranque vacío y que
`W` sea el del experimento.

### Todo de una vez

```powershell
.\scripts\todos-los-experimentos.ps1
```
```bash
./scripts/todos-los-experimentos.sh
```

### Uno por uno

| Experimento | Configuración | Comando |
|---|---|---|
| **E0** línea base | `W=1`, sin caché | `.\scripts\experimento.ps1 -Experimento E0` |
| **E1** con concurrencia | `W=8`, sin caché | `.\scripts\experimento.ps1 -Experimento E1` |
| **E2** concurrencia + caché | `W=8`, caché activo | `.\scripts\experimento.ps1 -Experimento E2 -Corrida run1` |

```bash
./scripts/experimento.sh E0
./scripts/experimento.sh E1
./scripts/experimento.sh E2 run1
./scripts/experimento.sh E2 run2   # el enunciado pide dos corridas de E2
```

La ráfaga por defecto son **128 solicitudes en 16 s** (8 req/s) rotando **10
tarjetas**, con tiempo límite de 3 s por solicitud. Cumple el numeral 7: al
menos 100 solicitudes en 15 a 20 segundos.

### Por qué E0 muestra el problema

Con `W=1` y 200 ms por cálculo, el servidor solo alcanza **5 solicitudes por
segundo**. La ráfaga llega a 8 req/s, así que la cola crece sin parar: la
latencia sube, el jitter se dispara y aparecen solicitudes que vencen su tiempo
límite. Con `W=8` la capacidad sube a 40 req/s y el problema desaparece. El
servidor imprime esa capacidad teórica al arrancar.

### Tabla de métricas

```powershell
.\scripts\generar-metricas.ps1
```
```bash
./scripts/generar-metricas.sh
```

Lee los CSV de `evidencia/` y escribe `evidencia/resultados.md` con las cuatro
medidas (latencia media y p95, throughput, jitter, eventos no procesados) más el
porcentaje de respuestas servidas desde caché.

### Resultados medidos

| Corrida | Latencia media | p95 | Throughput | Jitter | No procesadas | % caché |
|---|---:|---:|---:|---:|---:|---:|
| **E0** `W=1`, sin caché | 1.586,5 ms | 2.799,0 ms | 1,96 req/s | 75,6 ms | 91 de 128 | 0,0 % |
| **E1** `W=8`, sin caché | 202,3 ms | 203,0 ms | 7,96 req/s | 1,3 ms | 0 | 0,0 % |
| **E2 · run1** `W=8` + caché | 16,8 ms | 201,0 ms | 8,07 req/s | 1,8 ms | 0 | 92,2 % |
| **E2 · run2** `W=8` + caché | 16,8 ms | 202,0 ms | 8,06 req/s | 1,8 ms | 0 | 92,2 % |

En E2 una respuesta servida desde caché tarda **1,1 ms** frente a los **202,0 ms**
de una calculada. El análisis completo está en
[`docs/documentacion.md`](docs/documentacion.md).

---

## 5. Ver las tácticas en vivo

Con `LOG_PETICIONES=true` (el valor por defecto fuera de los experimentos), cada
consulta deja una línea que muestra si vino del caché:

```bash
docker compose logs -f servidor
```

```
recaudo-servidor | peticion GET /resumen/1000 -> 200 cache=MISS 201.4 ms 76 B cliente=172.19.0.1
recaudo-servidor | peticion GET /resumen/1001 -> 200 cache=MISS 202.1 ms 76 B cliente=172.19.0.1
recaudo-servidor | peticion GET /resumen/1000 -> 200 cache=HIT 0.2 ms 75 B cliente=172.19.0.1
```

Al principio todo es `MISS` y tarda ~200 ms; en cuanto las 10 tarjetas quedan
calculadas, pasa a `HIT` y a décimas de milisegundo.

Y mientras corre la ráfaga, `GET /config` muestra el pool trabajando:

```bash
curl http://localhost:8090/config
# {"trabajadores":8,"trabajadores_ocupados":8,"cola_actual":37,...,"cache_entradas":10,...}
```

| Variable | Defecto | Efecto |
|---|---|---|
| `TRABAJADORES` | `1` | W: cuántos cálculos corren a la vez |
| `COLA_MAX` | `256` | Solicitudes que pueden esperar turno antes de rechazar |
| `CACHE_ACTIVA` | `false` | Enciende o apaga la táctica R2 |
| `COSTO_CALCULO_MS` | `200` | Duración fija de un resumen calculado |
| `LOG_PETICIONES` | `true` | Una línea por consulta (`cache=HIT/MISS`) |
| `LOG_VIGILANCIA` | `false` | Añade el tráfico de inspección: `/config` y `/salud` |

Los scripts apagan `LOG_PETICIONES` durante las corridas: escribir una línea por
solicitud mientras se mide la latencia contamina justo lo que se quiere medir.

---

## 6. Pruebas automatizadas

```powershell
.\scripts\pruebas.ps1 -Race
```
```bash
./scripts/pruebas.sh -race
```

Si Go no está instalado, los scripts usan la imagen `golang:1.21-alpine`. Las
pruebas comprueban, entre otras cosas, que:

- con `W=1` nunca hay dos cálculos simultáneos y con `W=8` sí se llega a 8;
- más trabajadores terminan la misma ráfaga en menos tiempo;
- la segunda consulta de una tarjeta viene del caché sin recalcular;
- un acierto de caché **no** pasa por el pool;
- el caché no altera el resultado frente a recalcular;
- con la cola llena el servidor responde `503`;
- el cálculo respeta el tiempo fijo configurado (restricción 1).

---

## 7. Estructura del proyecto

```
desempeno/
├── cmd/
│   ├── servidor/       proceso único que atiende /resumen
│   ├── cliente/        script de ráfaga -> CSV de evidencia
│   └── metricas/       CSV -> tabla de las cuatro medidas
├── internal/
│   ├── domain/         entidades y reglas puras (Resumen, ClaveResumen, tarifas)
│   ├── usecase/        pool de trabajadores (R1) + consulta con caché (R2)
│   ├── adapter/
│   │   ├── httpin/     API HTTP de entrada
│   │   ├── cache/      caché en memoria y caché apagado
│   │   └── calculo/    el cálculo costoso
│   ├── infra/          configuración, servidor HTTP, composition root
│   └── metricas/       análisis del CSV
├── deploy/             Dockerfiles
├── scripts/            experimentos y pruebas (.ps1 y .sh)
├── evidencia/          CSV, tabla de resultados
└── docs/               documentación del taller
```

La dependencia siempre apunta hacia adentro: `domain` no importa nada;
`usecase` solo importa `domain` y sus puertos; los adaptadores implementan esos
puertos; `infra/ensamblaje` es el único lugar donde se decide qué
implementación concreta se usa (por eso apagar el caché es cambiar una variable
y no tocar el caso de uso).

---

## 8. Entregables del taller

| Entregable | Dónde |
|---|---|
| 1. Código fuente con arranque en un comando | Este repositorio · `docker compose up --build` |
| 2. CSV del cliente de E0, E1 y E2 | [`evidencia/`](evidencia/) |
| 3. Tabla con las cuatro métricas y el % desde caché | [`evidencia/resultados.md`](evidencia/) |
| 4. Respuestas a las preguntas de análisis | [`docs/`](docs/) |
