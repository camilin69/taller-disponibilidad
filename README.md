# RECAUDO-T · Talleres de arquitectura de software

Monorepo con las implementaciones de RECAUDO-T para distintos talleres de
tácticas de calidad. Cada carpeta es un proyecto independiente: su propio
módulo de Go, su propio `docker-compose.yml` y su propia evidencia. Se pueden
levantar por separado o al mismo tiempo (los puertos no chocan).

| Proyecto | Tácticas | Arranca en | Puertos |
|---|---|---|---|
| [`disponibilidad/`](disponibilidad/) | Ping/Echo (detección de fallas) + Redundancia Activa | `cd disponibilidad && docker compose up --build` | 8080 – 8083, 3000 |
| [`desempeno/`](desempeno/) | Introducir Concurrencia + Mantener Múltiples Copias de Datos (caché) | `cd desempeno && docker compose up --build` | 8090 |

Cada carpeta tiene su propio README con el detalle: escenario de calidad,
arquitectura, cómo correr los experimentos y los resultados medidos.

---

## Levantar los dos a la vez

```bash
cd disponibilidad && docker compose up -d --build && cd ../desempeno && docker compose up -d --build
```

```bash
curl http://localhost:8080/estado                       # disponibilidad
curl "http://localhost:8090/resumen/1234?viajes=40"      # desempeno
```

## Qué comparten

Ambos proyectos parten del mismo servicio de negocio, **RECAUDO-T**, y siguen
las mismas convenciones: Go 1.21 con solo biblioteca estándar, Clean
Architecture (`domain` → `usecase` → `adapter`/`infra`), Docker Compose para
orquestar, y un cliente de carga que deja su evidencia en CSV dentro de
`evidencia/`. Lo que cambia entre ellos es la táctica de calidad que atacan:
disponibilidad (seguir funcionando ante una falla) vs. desempeño (responder a
tiempo bajo carga).

El archivo [`.gitattributes`](.gitattributes) queda en la raíz porque sus
reglas de fin de línea (`eol=lf` / `eol=crlf`) aplican a todo el repositorio,
no solo a un proyecto.
