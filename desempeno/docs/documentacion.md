# Taller de tácticas de desempeño · RECAUDO-T

**Tácticas implementadas:** Introducir Concurrencia (R1) y Mantener Múltiples
Copias de Datos / caché (R2), ambas de la rama *Gestionar los recursos*.

---

## 1. Escenario de calidad

| Parte del escenario | Valor |
|---|---|
| Fuente del estímulo | Usuarios (muchas solicitudes simultáneas desde la aplicación móvil) |
| Estímulo | Ráfaga de solicitudes concurrentes al resumen mensual: 128 solicitudes en 16 s |
| Artefacto | Servicio de resumen mensual de viajes de RECAUDO-T |
| Entorno | Operación normal, al inicio de mes (pico de consultas) |
| Respuesta | El sistema sigue respondiendo en un tiempo aceptable y no descarta solicitudes |
| Medida de la respuesta | Latencia p95 dentro de un límite razonable · throughput sostenido · sin solicitudes descartadas |

Las solicitudes de este taller son **esporádicas**: llegan en ráfagas
impredecibles, como ocurre al inicio de un mes real.

---

## 2. Decisiones de diseño

### El cálculo costoso es real y de duración fija

`internal/adapter/calculo/viajes.go` recorre los viajes del mes uno por uno y
acumula el gasto (trabajo real y determinista), y luego rellena hasta completar
`COSTO_CALCULO_MS` (200 ms). Ese relleno modela el costo de **leer** el
historial de la base de datos, que en el servicio real es espera de E/S, no CPU.

Al fijar la duración total, todo cálculo tarda lo mismo (restricción 1 del
enunciado), de modo que las diferencias entre E0, E1 y E2 vienen de las tácticas
y no del ruido del cálculo.

### El pool es explícito, no delegado al framework

`net/http` de Go arranca una goroutine por petición. Si el cálculo ocurriera
dentro del manejador HTTP, el servidor ya sería concurrente sin que nadie
configure nada y **`W=1` sería imposible de medir**: eso es justo lo que la
restricción 1 prohíbe.

Por eso el cálculo solo ocurre dentro del pool de `internal/usecase/pool.go`: un
conjunto fijo de `W` goroutines que consumen de un canal acotado. El manejador
HTTP encola y espera; `W` es el límite real de paralelismo del servicio.

### El caché se consulta antes de encolar

En `internal/usecase/obtener_resumen.go` el caché se consulta **antes** del
pool. Ese orden es la razón de ser de la táctica: si el caché se consultara
dentro del trabajador, un acierto seguiría haciendo fila detrás de los cálculos
y dejaría de ser instantáneo. Una prueba automatizada
(`TestAciertoDeCacheNoOcupaTrabajadores`) fija esa garantía.

La clave es `(id_tarjeta, viajes)`, no solo la tarjeta: el resumen de los
últimos 40 viajes no es el mismo que el de los últimos 60. Cachear solo por
tarjeta devolvería un total equivocado.

### Apagar una táctica es cambiar una variable

`CACHE_ACTIVA=false` no pone un `if` dentro del caso de uso: hace que el
composition root inyecte `cache.Desactivada`, una implementación del mismo
puerto que nunca acierta. El caso de uso no sabe si el caché está encendido.

---

## 3. Protocolo experimental

| Experimento | W | Caché | Qué muestra |
|---|---:|---|---|
| **E0** | 1 | no | La línea base: el problema tal como llegó |
| **E1** | 8 | no | El efecto de la concurrencia sola |
| **E2** | 8 | sí | El efecto de las dos tácticas juntas (dos corridas) |

Ráfaga: 128 solicitudes en 16 s (8 req/s) rotando 10 tarjetas, con tiempo límite
de 3 s por solicitud.

**Por qué E0 satura.** Con `W=1` y 200 ms por cálculo, la capacidad teórica del
servidor es de 5 solicitudes por segundo. La ráfaga llega a 8 req/s, así que
cada segundo entran 3 solicitudes más de las que salen: la cola crece sin parar,
la latencia sube linealmente y a partir de cierto punto las solicitudes vencen
su tiempo límite de 3 s. Con `W=8` la capacidad sube a 40 req/s, muy por encima
de la ráfaga, y el problema desaparece.

---

## 4. Resultados

Corridas reales del 22 de septiembre de 2026. Cada número sale del CSV que se
nombra; la tabla completa está en [`evidencia/resultados.md`](../evidencia/resultados.md).

| Corrida | Evidencia | Solicitudes | Latencia media | Latencia p95 | Throughput | Jitter | No procesadas | % desde caché |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| **E0** | `e0_cliente.csv` | 128 | 1586,5 ms | 2799,0 ms | 1,96 req/s | 75,6 ms | 91 (71,1 %) | 0,0 % |
| **E1** | `e1_cliente.csv` | 128 | 202,3 ms | 203,0 ms | 7,96 req/s | 1,3 ms | 0 (0,0 %) | 0,0 % |
| **E2 · run1** | `e2_run1_cliente.csv` | 128 | 16,8 ms | 201,0 ms | 8,07 req/s | 1,8 ms | 0 (0,0 %) | 92,2 % |
| **E2 · run2** | `e2_run2_cliente.csv` | 128 | 16,8 ms | 202,0 ms | 8,06 req/s | 1,8 ms | 0 (0,0 %) | 92,2 % |

### Efecto del caché

| Corrida | Desde caché | Calculadas | Latencia media desde caché | Latencia media calculada |
|---|---:|---:|---:|---:|
| E2 · run1 | 118 | 10 | **1,1 ms** | 202,0 ms |
| E2 · run2 | 118 | 10 | **1,1 ms** | 202,2 ms |

Las dos corridas de E2 coinciden hasta la décima de milisegundo: los valores
están confirmados.

### Lo que muestra E0: colapso por congestión

El CSV de E0 tiene un patrón limpísimo. Las solicitudes **1 a 37 tuvieron
éxito** y las **38 a 128 fallaron**, todas exactamente a los 3.000 ms. La
latencia crece de forma lineal:

| Secuencia | 1 | 9 | 17 | 25 | 33 | 41 |
|---|---:|---:|---:|---:|---:|---:|
| Latencia | 228 ms | 827 ms | 1.432 ms | 2.041 ms | 2.648 ms | 3.001 ms ❌ |

Cada solicitud espera **75 ms más que la anterior**, y ese número no es
casualidad: es exactamente `1/5 − 1/8` de segundo, el déficit entre la
capacidad del servidor (5 req/s) y la demanda (8 req/s). Es también, casi al
milímetro, el jitter medido: **75,6 ms**. El jitter de la teoría resultó ser una
medición directa del ritmo al que se degrada el servicio.

Hay un segundo efecto, menos obvio y más grave. El throughput de E0 fue de
**1,96 req/s**, bastante por debajo de la capacidad teórica de 5 req/s. La razón
es que, pasado el segundo 5, el único trabajador dedica sus 200 ms a
solicitudes que ya casi agotaron su presupuesto de 3 s: termina el cálculo y el
cliente ya se rindió. El servidor está al 100 % de ocupación produciendo
resultados que nadie recibe. Eso es **colapso por congestión**: más allá de
cierto punto, la carga extra no solo no se atiende, sino que destruye la
capacidad de atender el resto.

Es el argumento más fuerte a favor de acotar la cola y de descartar el trabajo
abandonado: el pool ya descarta las tareas cuyo cliente se rindió *antes* de
empezarlas, pero no puede adivinar cuáles se rendirán *durante* el cálculo. La
defensa completa sería rechazar por plazo (si la espera estimada supera el
presupuesto del cliente, responder `503` de inmediato en vez de encolar).

---

## 5. Preguntas de análisis

### 1. Compare el throughput de E0 y E1. ¿Por qué aumentar el número de trabajadores mejora el throughput aunque cada cálculo individual siga tardando lo mismo?

Medimos **1,96 req/s en E0** y **7,96 req/s en E1**: el throughput se
cuadruplicó (×4,06) con el mismo cálculo de 200 ms.

Porque el throughput no depende de cuánto tarda **un** cálculo, sino de cuántos
caben **a la vez**. El cálculo sigue costando 200 ms en los dos experimentos;
lo que cambia es cuántos de esos 200 ms transcurren en paralelo.

Con `W=1` el servidor procesa una solicitud tras otra: su techo es
`1 / 0,2 s = 5 req/s`, sin importar cuántas lleguen. Con `W=8` hay ocho cálculos
corriendo simultáneamente, así que el techo sube a `8 / 0,2 s = 40 req/s`. La
fórmula es `throughput = W / costo`: el costo quedó igual, el numerador se
multiplicó por ocho.

Nótese que E1 no llegó a 40 req/s sino a 7,96: **el límite dejó de ser el
servidor**. La ráfaga solo envía 8 req/s, así que el servidor atiende todo lo
que le llega y sobra capacidad. Ese es justamente el objetivo: que el cuello de
botella deje de estar en el servicio. E0, en cambio, ni siquiera alcanzó sus
5 req/s teóricos (dio 1,96) por el colapso por congestión que se explica en el
numeral 4.

Hay una razón física detrás: el costo de este cálculo es sobre todo **espera**
(leer registros del historial), no CPU quemada. Mientras un trabajador espera su
lectura, el procesador está libre para que otro avance. La concurrencia no hace
que nada vaya más rápido: hace que los tiempos muertos se solapen en vez de
sumarse.

### 2. En E1, ¿bajó la latencia promedio de cada solicitud individual, o solo mejoró cuántas solicitudes se atendieron en total? Explique la diferencia entre esos dos efectos.

Bajó la latencia promedio —de **1.586,5 ms a 202,3 ms**— pero **no porque el
cálculo se hiciera más rápido**. Conviene separar la latencia en dos partes:

```
latencia = tiempo en la cola + tiempo de cálculo
```

El **tiempo de cálculo** es idéntico en E0 y E1: 200 ms, por diseño. Lo que se
desplomó es el **tiempo en la cola**. En E0 la cola crecía sin control, así que
la solicitud número 33 esperaba 2.448 ms antes de empezar siquiera a
procesarse. En E1 el servidor drena la cola más rápido de lo que llega, así que
casi ninguna solicitud espera: la latencia media de 202,3 ms es el piso del
cálculo más 2,3 ms de todo lo demás.

La evidencia más nítida de esto es el **jitter**, que cayó de **75,6 ms a
1,3 ms**. En E0 cada solicitud esperaba sistemáticamente más que la anterior
(servicio impredecible, degradándose); en E1 todas tardan prácticamente lo
mismo. Y las **no procesadas pasaron de 91 a 0**: la respuesta esperada del
escenario de calidad —«no descarta solicitudes»— se cumple.

La diferencia entre los dos efectos importa: **el throughput es una propiedad
del sistema** (cuántas solicitudes despacha por segundo) y **la latencia es una
propiedad de cada solicitud** (cuánto esperó ese usuario). Mejorar el throughput
mejora la latencia *indirectamente*, al evitar que se forme cola. Si la carga
subiera hasta volver a superar la capacidad (más de 40 req/s con `W=8`), el
throughput seguiría en su techo pero la latencia volvería a dispararse. El piso
de 200 ms, en cambio, la concurrencia no lo puede tocar: para eso hace falta
otra táctica, y esa es el caché.

### 3. En E2, ¿qué porcentaje de solicitudes provino del caché? ¿Qué pasaría con ese porcentaje si el cliente usara 1000 tarjetas distintas en vez de 10? ¿Sigue siendo útil el caché en ese caso?

**92,2 % en las dos corridas** (118 de 128), con una latencia media de
**1,1 ms** frente a los **202,0 ms** de las 10 que sí se calcularon: un factor
de 180×.

Ese 92,2 % es exactamente el techo teórico, `(128 − 10) / 128`. Solo se calculó
una vez cada tarjeta: ninguna se recalculó. Vale la pena entender por qué,
porque no estaba garantizado. Con 10 tarjetas a 8 req/s, una tarjeta se repite
cada 1,25 s, y el cálculo tarda 0,2 s: cuando llega la segunda consulta de una
tarjeta, la primera ya terminó y dejó su resultado guardado.

Si la ráfaga fuera más rápida o las tarjetas menos —digamos 40 req/s sobre 10
tarjetas, una repetición cada 0,25 s— varias solicitudes de la misma tarjeta
llegarían antes de que la primera termine, todas verían el caché vacío y todas
calcularían. Es el problema del *thundering herd*. Se resuelve con
*single-flight*: hacer que la segunda solicitud de una tarjeta que ya se está
calculando espere ese resultado en vez de lanzar un cálculo duplicado. El taller
no lo pide y esta configuración no lo necesita, pero es la mejora natural si la
carga sube.

Nótese también que el **p95 de E2 es 201,0 ms**, prácticamente igual al de E1,
mientras que la media es de 16,8 ms. No es contradictorio: el 92 % que acierta
responde en ~1 ms y el 8 % que calcula sigue costando 200 ms, así que el
percentil 95 cae dentro de ese 8 %. El caché mejora muchísimo el caso típico y
no toca el peor caso: para bajar el p95 habría que atacar el costo del cálculo
en sí.

**Con 1000 tarjetas distintas** y 128 solicitudes, no habría ninguna repetición:
el porcentaje de aciertos caería a **0 %** y el caché sería pura sobrecarga
(memoria y una consulta fallida antes de cada cálculo). El resultado se parecería
al de E1: 202 ms de media, sin el beneficio del caché pero con su costo.

**¿Sigue siendo útil?** En este experimento no, pero en el servicio real sí, por
dos razones. Primera: la utilidad del caché no depende del número de tarjetas
sino de la **tasa de repetición**. Con 1000 tarjetas y 100.000 solicitudes al
día habría 100 consultas por tarjeta y el caché volvería a acertar casi siempre.
Segunda: al inicio de mes el patrón real es exactamente ese — mucha gente
consulta su resumen, y buena parte lo consulta **varias veces en la misma
sesión**. Lo que mataría al caché no es tener muchas tarjetas, sino que cada
consulta fuera única e irrepetible.

Con un universo grande sí habría que agregar una política de expulsión (LRU y un
tope de entradas): un caché sin límite sobre 1000 tarjetas crece hasta agotar la
memoria del proceso.

### 4. Si el servidor aumentara indefinidamente el número de trabajadores, ¿qué límite físico o técnico lo detendría con el tiempo?

El throughput **no** crece indefinidamente con `W`; la curva se aplana y luego
cae. Los límites, en el orden en que aparecen:

1. **Núcleos de CPU.** Mientras el costo sea espera de E/S, muchos trabajadores
   caben por núcleo. Pero la parte de CPU del cálculo (recorrer y sumar los
   viajes) sí compite: pasado cierto punto los trabajadores se turnan el
   procesador y cada uno tarda más.
2. **Cambio de contexto.** Cada trabajador adicional obliga al planificador a
   repartir entre más candidatos. Con miles de trabajadores, una fracción
   creciente del tiempo se va en cambiar de contexto en vez de calcular.
3. **Memoria.** Cada trabajador tiene su pila y su estado en curso. Con
   suficientes, el proceso agota la RAM.
4. **El recurso compartido de aguas abajo.** El límite que suele llegar primero
   en un sistema real: la base de datos de la que se leen los viajes tiene su
   propio pool de conexiones. Duplicar los trabajadores del servidor no duplica
   la capacidad de la base de datos; solo traslada la cola de un sitio a otro.

Esto conecta con la táctica **Aumentar los Recursos** del árbol. "Introducir
concurrencia" exprime mejor el hardware que ya se tiene: es gratis hasta que se
topa con el techo físico. Pasado ese punto, la única salida es *aumentar los
recursos*: más núcleos, más memoria, más máquinas (escalamiento horizontal) o
una base de datos más capaz. Son tácticas complementarias, y en ese orden:
primero se usa bien lo que hay, y solo después se compra más.

### 5. El caché evita repetir el cálculo, pero ¿qué pasa si el usuario realiza un nuevo viaje después de que su resumen quedó guardado en caché?

Aparece un problema de **coherencia**: el caché devuelve un resumen obsoleto. El
usuario acaba de pagar un pasaje, abre la app y ve el gasto de antes del viaje.
Para él es un error de datos, aunque el sistema esté funcionando como se diseñó.

Es el costo de la táctica: el caché cambia *frescura* por *velocidad*. Mientras
los datos no cambien, el trato es gratis; en cuanto cambian, hay que decidir
cuánta desactualización se tolera. En este taller la restricción 2 dice que el
caché no necesita expirar, y eso es válido **solo porque durante el experimento
nadie registra viajes nuevos**.

Para resolverlo habría que agregar una de estas tres cosas:

1. **Invalidación por evento (lo correcto).** Cuando se registra un viaje, el
   servicio borra la entrada de esa tarjeta. La siguiente consulta recalcula.
   Da datos siempre correctos, a cambio de acoplar el registro de viajes con el
   caché: quien escribe tiene que avisarle a quien cachea.
2. **Expiración por tiempo (TTL).** Cada entrada vive, por ejemplo, 60 s. Es
   trivial de implementar y no acopla nada, pero acepta una ventana de datos
   viejos: el usuario podría ver su gasto desactualizado hasta un minuto.
3. **Escritura a través del caché** (*write-through*): quien registra el viaje
   actualiza el resumen guardado sumándole la tarifa, en vez de borrarlo. Evita
   recalcular, pero exige que la actualización incremental sea equivalente al
   cálculo completo.

Para un resumen de gastos, la invalidación por evento es la respuesta adecuada:
el usuario espera ver su viaje reflejado de inmediato, y los viajes se registran
con mucha menos frecuencia de lo que se consultan los resúmenes, así que
invalidar cuesta poco.

Hay además un detalle que el caché de este taller ya evita: como la clave
incluye el número de viajes, un resumen de 40 viajes nunca se sirve a quien pide
60. Sin eso habría un segundo problema de coherencia, y más difícil de
diagnosticar que el del viaje nuevo.

---

## 6. Verificación antes de entregar

| Comprobación | Cómo |
|---|---|
| El servidor responde | `curl "http://localhost:8090/resumen/1234?viajes=40"` |
| Hay diferencia medible de throughput entre W=1 y W=8 | Tabla de `evidencia/resultados.md` (E0 vs E1) |
| `desde_cache` distingue calculadas de servidas del caché | Columna `desde_cache` en los CSV de E2 |
| Los trabajadores y el caché están donde se dice | `curl http://localhost:8090/config` durante la ráfaga |
| Las garantías del diseño no se rompen | `./scripts/pruebas.sh -race` |
