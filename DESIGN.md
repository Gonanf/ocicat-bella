# Ocicat Bella — Documento de Diseño Técnico

> Diseño vivo. Se completa por partes.
> Backend en Go (binario único), frontend en framework JS separado. FOSS, auto-host, auditable.
> Redactado por Kateto (especificaciones y decisiones, no código).

---

## 0. Principios, Infraestructura, Producto y Arquitectura (definido por el usuario)

Esta sección son decisiones de producto/infra/arquitectura, no código.

### 0.1 Infraestructura y despliegue
- **Backend = binario Go estático único** (`CGO_ENABLED=0`). Se copia y corre. Sin
  runtime ni dependencias del sistema. No-negociable para el target (servidores viejos).
- **Imagen mínima:** el backend corre en un contenedor distroless <50MB. Misma imagen
  para self-host (escuela) y para el SaaS hosteado por el autor. Cambia solo quién la corre.
- **Cloudflare:** el frontend estático (Astro) puede ir 100% a Cloudflare Pages. El backend
  con Docker NO corre en Workers (no hay Docker socket). Cloudflare sostiene borde/estáticos/
  dominio/TLS; el compute pesado queda en un VPS chico. Cloudflare es opcional, no obligatorio.
- **Setup AMIGABLE (visual, no CLI):** el binario arranca y abre un **wizard web local**
  (`http://localhost:8484/setup`) con pantallas guiadas, NO comandos en terminal. Justificación:
  el usuario puede no saber terminal y puede usar Windows. El wizard resuelve: "¿tenés Docker?
  → lo instalo por vos", "nombre de la escuela", "¿querés federarte?". En Windows el onboarding
  es un **instalador .exe/.msi** que descarga el binario, instala Docker Desktop si falta y abre
  el wizard en el navegador. El CLI (`ocicat init/up`) queda solo para quien ya sabe y para el
  SaaS headless. Objetivo: que cualquier profe lo levante sin tocar la terminal.

### 0.2 Producto: agradable, amigable, divertido
- **Affordances familiares:** materiales estilo Drive (carpetas, tarjetas, búsqueda);
  cursos/proyectos estilo Classroom (stream, entregas). No inventar metáforas nuevas.
- **Diferencial tangible:** el alumno sube su proyecto y ve "▶ Ejecutar" con logs en vivo.
  Eso es el "wow" y lo que Classroom no hace.
- **Tono y accesibilidad:** UI ligera (no SPA que come 500MB RAM), amigable, sin jerga,
  que ande en PCs viejos con pantallas chicas.

### 0.3 Utilidad real — fuente de la verdad de contenido
- **Ocicat es la fuente de la verdad de proyectos y documentos de CUALQUIER materia**, no solo
  programación. Literatura sube su libro/informe; arte sube sus dibujos; programación sube sus
  proyectos andando (en contenedor). Se pueden subir imágenes, dibujos, tareas, videos, etc.
- **No maneja alumnos ni entregas de tareas** → por eso NO reemplaza Classroom. Es contenido,
  no gestión académica. Frontera de producto explícita.
- **Diferencial:** el contenedor aislado (programación) + la federación de TODO el contenido
  entre escuelas. Un alumno de arte de la técnica ve el dibujo del de otra provincia; el de
  literatura ve el informe del de UTN. Comunidad de producción, no solo storage.
- **Posicionamiento:** "la fuente de la verdad de lo que producen las escuelas, ejecutable y
  compartible entre instituciones". El módulo de materiales es el core, no un complemento.

### 0.4 Arquitectura: monolito modular en capas + eventos + plugins opcionales
- **NO microservicios:** las escuelas tienen un server viejo; correr N servicios con
  orquestación mata al server y a quien lo mantiene. Microservicios resuelven escala
  organizacional, acá es un equipo chico. Complejidad sin beneficio.
- **Capas internas:** `api → service → runner/store/federation`. El runner tras una
  interfaz (`SandboxBackend`), por eso Docker→Firecracker no rompe la API.
- **Eventos internos:** el Runner emite eventos (`run.started`, `run.log`, `run.finished`,
  `run.cleaned`) por un bus en-process (canal de Go). La API los expone por SSE. Desacopla
  "ejecutar" de "mostrar logs" sin microservicios.
- **Plugins (post-MVP):** hook points para extender (notificar Discord, calificar desde
  Classroom). Se cargan como binarios/WASM separados. El core no depende de ellos.
- **Conclusión:** un solo binario, modular por dentro, comunicación por eventos, extensible
  por plugins. Sirve al constraint de server viejo + personal no experto + eficiencia.

---

### 0.5 Persistencia — `store` es una interfaz; MVP usa Turso (LibSQL)
- **MVP = Turso / LibSQL** (SQLite-compatible, con replicación/sync nativa). No hay buena razón
  para usar SQLite puro en el MVP: Turso es lo mismo pero ya trae lo que necesitamos para
  federación (replicación de datos) sin inventar un protocolo. La federación deja de ser un
  problema de red y pasa a ser config de replicación.
- **SQLite** queda como motor embedded de respaldo para quien no quiera red (un solo archivo).
- **Limbo** (DB en Rust, compatible SQLite): post-MVP si se quiere ecosistema Rust.
- **DuckDB**: solo analytics/reportes de uso de la escuela (OLAP), no OLTP.
- **PostgreSQL**: escuelas grandes con concurrencia alta / multi-instancia (Fase 3), detrás de
  la misma interfaz `Store`.
- **Decisión:** la capa `internal/store` expone `Store`; el motor se elige por config. MVP trae
  Turso; los demás son backends intercambiables sin tocar API ni Runner.

### 0.6 Identidad, permisos y abuso (definido con el usuario)
- **Docente: OAuth de Google.** Ya usan Classroom, así que no crean otra cuenta. El docente
  autenticado crea salas y es admin de las mismas.
- **Alumno: entra por CÓDIGO DE SALA, no por cuenta.** El docente genera una sala y un código
  corto (ej. `OCI-7F3K`) + URL `ocicat.edu/s/OCI-7F3K`. El alumno entra por la URL o tipea el
  código en `/join`. El código es el "token de sala": llave de acceso al contexto, no una cuenta.
- **Identidad de alumno (definido con el usuario):** al entrar a una sala, el backend emite un
  **token de sesión de alumno** local a esa sala (anon, rastreable dentro de la sala). Eso da un
  `Alumno` local con `sala_id` y un alias (ej. `Alumno-7F3K-12`). `Sandbox.AutorID` apunta a ese
  registro → permite filtrar "proyectos de este alumno" y suspender el token si abusa. NO hay
  bloqueo por IP ni por dispositivo: si el alumno sale y vuelve a entrar a la sala, le toca otro
  token de alumno local. Es identidad efímera por sala, no global.
- **Nombre/email OPCIONAL:** el alumno puede (no debe) poner nombre y email para que lo busquen
  por nombre. Sin eso, queda como alias anónimo de la sala. Esto resuelve trazabilidad sin obligar
  a cuenta y sin romper adopción.
- **Visibilidad (definido con el usuario):** TODOS los alumnos pueden VER todo (su sala, otras
  salas de su escuela, y sandboxes federados de otras escuelas, en lectura). Pero solo pueden
  SUBIR proyectos/documentos en la sala que les permiten (la que entraron por código). Lectura
  global, escritura por sala. Esto maximiza la federación (descubrimiento) sin abrir la puerta a
  que cualquiera escriba en la sala de otro.
- **Seats del código:** por defecto **código abierto** (cualquiera con el código entra; el profe
  lo pasa en el pizarrón; asientos infinitos). Opción de **capar seats** (el profe define N cups,
  ej. 30) si quiere acotar. El RateLimiter del Runner (§2.4) ya frena abuso de sandboxes por sala,
  así que no hace falta seats para proteger el server — es solo control de la sala.
- **Permisos mínimos:** dueño del sandbox (quien lo sube) + lectores (la sala) + docente admin.
  Sin roles corporativos pesados.
- **Suspensión de alumno abusivo (definido con el usuario):** el alumno entra por código, sin
  cuenta, así que NO se bloquea por IP ni por dispositivo (IP compartida en la escuela mataría a
  todo el curso; el dispositivo se reinicia y vuelve). Mecanismos reales:
  1. **Rotación de código de sala:** el docente (admin) regenera el código de la sala con un botón.
     El abusivo, sin el nuevo código, queda afuera. Es el equivalente escolar a "cambiar la
     contraseña"; simple y sin infra.
  2. **Vetar nombre/email:** si el alumno se identificó (opcional, §0.6), el docente puede vetar
     ese nombre/email para futuros ingresos a la sala. Tiene efecto duradero porque la identidad es
     la que dio.
  3. **Rate limiter del Runner (§2.4)** ya frena el abuso de sandboxes por sala sin identificar al
     individuo: el daño (server colgado) está cubierto por infra, no por auth.
- **Conclusión:** docente OAuth Google + alumno por código de sala + visibilidad híbrida + límites
  de cuota + rate limiter. Adopción alta (el pibe no crea cuenta) y spam frenado.

### 0.7 Siguiente paso: diseño visual con open-design
Una vez cerrado el diseño (Partes 4–7), se pasa a diseño visual con
https://github.com/nexu-io/open-design. El DESIGN.md es la especificación; open-design produce
los mockups de las pantallas (landing, /materials, /sandboxes, /run, /join, /setup wizard).

---

## 1. Visión general y topología

Ocicat Bella son **dos servicios separados** que se hablan por HTTP/JSON:

```
[ Navegador ] ──HTTP/JSON──> [ Frontend JS ] ──HTTP/JSON (API)──> [ Backend Go ]
                                                                    │
                                                        ┌───────────┼────────────┐
                                                  [ Runner ]    [ Store ]   [ Federation ]
                                                   (Docker/        (SQLite)     (P2P/relay)
                                                   Firecracker)
                                                        │
                                                  [ Contenedores aislados de proyectos ]
```

- **Backend Go**: binario único. Expone la API, orquesta el Runner, persiste en SQLite, habla con peers de federación.
- **Frontend JS**: servicio aparte (Astro + daisyUI). Consume la API. Se puede reemplazar sin tocar el backend.
- **Separación estricta**: el frontend NUNCA toca Docker ni la DB directo. Solo la API.

---

## 2. Estructura de carpetas (monorepo)

```
ocicat-bella/
├── backend/                  # Go module (github.com/ocicat/backend)
│   ├── cmd/
│   │   └── server/
│   │       └── main.go        # arranque: carga config, migrations, listen
│   ├── internal/
│   │   ├── api/               # handlers HTTP (materiales, proyectos, run, federation)
│   │   │   ├── material.go
│   │   │   ├── project.go
│   │   │   ├── run.go
│   │   │   └── federation.go
│   │   ├── runner/            # abstracción de sandbox
│   │   │   ├── backend.go      # interface SandboxBackend
│   │   │   ├── docker.go       # impl Docker (Fase 1)
│   │   │   ├── firecracker.go  # impl Firecracker (Fase 3, stub)
│   │   │   ├── limits.go       # struct de límites
│   │   │   └── lifecycle.go    # máquina de estados
│   │   ├── store/             # persistencia (SQLite + sqlc/sqlx)
│   │   │   ├── db.go
│   │   │   ├── models.go       # structs Alumno, Material, Sandbox, Sala, ContainerRun, FederationPeer
│   │   │   └── migrations/     # .sql up/down
│   │   ├── federation/        # protocolo P2P + relay
│   │   │   ├── protocol.go     # tipos de mensaje
│   │   │   ├── peer.go         # gestión de peers
│   │   │   ├── discovery.go    # DHT simple o relay
│   │   │   └── reconcile.go    # MERGE de metadatos
│   │   └── config/            # config desde env + archivo
│   │       └── config.go
│   ├── go.mod
│   └── Dockerfile             # imagen del backend
├── frontend/                 # framework JS (Astro/Nuxt/Vue) — repo aparte
│   ├── src/
│   │   ├── lib/ocicat-api.ts  # cliente tipado de la API
│   │   ├── types.ts           # tipos que espejan el backend
│   │   ├── pages/             # catálogo, proyectos, ejecución, federación
│   │   └── components/        # Shadcn + los propios
│   ├── package.json
│   └── astro.config.ts (o nuxt.config.ts)
├── docker/                   # plantillas de proyectos de alumnos
│   ├── python/Dockerfile
│   ├── node/Dockerfile
│   └── go/Dockerfile
├── DESIGN.md
├── STACK.md
└── README.md
```

---

## 3. Modelo de datos

### 3.1 Go structs (`internal/store/models.go`)

```go
package store

import "time"

type Alumno struct {
    ID        int64     `db:"id" json:"id"`
    Nombre    string    `db:"nombre" json:"nombre"`
    Email     string    `db:"email" json:"email"`
    Escuela   string    `db:"escuela" json:"escuela"`
    CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Material struct {
    ID          int64     `db:"id" json:"id"`
    Titulo      string    `db:"titulo" json:"titulo"`
    Tipo        string    `db:"tipo" json:"tipo"` // libro | doc | video
    URL         string    `db:"url" json:"url"`   // o path si es local
    AutorID     int64     `db:"autor_id" json:"autor_id"`
    CreadoEn    time.Time `db:"creado_en" json:"creado_en"`
}

type Sandbox struct {
    ID          int64     `db:"id" json:"id"`
    Nombre      string    `db:"nombre" json:"nombre"`
    Lenguaje    string    `db:"lenguaje" json:"lenguaje"` // python | node | go
    RepoURL     string    `db:"repo_url" json:"repo_url"` // git del alumno
    AutorID     int64     `db:"autor_id" json:"autor_id"`
    Visibility  string    `db:"visibility" json:"visibility"` // private | federated
    SalaID      int64     `db:"sala_id" json:"sala_id"` // sala/clase a la que pertenece
    Modo        string    `db:"modo" json:"modo"` // "job" (playground efímero) | "service" (server vivo, ej. PHP)
    Servicio    string    `db:"servicio" json:"servicio"` // modo service: puerto+ruta/url asignada
    Retention   string    `db:"retention" json:"retention"` // "historical" (por defecto) | "ephemeral" (alumno se niega a guardar)
    CreatedAt   time.Time `db:"created_at" json:"created_at"`
}

// Estado de una ejecución aislada
type RunStatus string

const (
    RunPending   RunStatus = "pending"
    RunRunning   RunStatus = "running"
    RunSucceeded RunStatus = "succeeded"
    RunFailed    RunStatus = "failed"
    RunCleaned   RunStatus = "cleaned"
)

type ContainerRun struct {
    ID         int64     `db:"id" json:"id"`
    SandboxID  int64     `db:"sandbox_id" json:"sandbox_id"` // FK a Sandbox
    SchoolID   int64     `db:"school_id" json:"school_id"` // para presupuesto del RateLimiter (§2.4)
    SalaID     int64     `db:"sala_id" json:"sala_id"`
    Backend    string    `db:"backend" json:"backend"` // docker | firecracker | unikernel
    Status     RunStatus `db:"status" json:"status"`
    ExtID      string    `db:"ext_id" json:"ext_id"` // id del contenedor/VM en el backend
    StartedAt  time.Time `db:"started_at" json:"started_at"`
    FinishedAt *time.Time `db:"finished_at" json:"finished_at,omitempty"`
    ExitCode   *int      `db:"exit_code" json:"exit_code,omitempty"`
}

// Peer de federación
type FederationPeer struct {
    ID        int64     `db:"id" json:"id"`
    NodeID    string    `db:"node_id" json:"node_id"` // UUID del peer
    Name      string    `db:"name" json:"name"`       // "UTN Córdoba"
    Endpoint  string    `db:"endpoint" json:"endpoint"` // https://... o relay://
    LastSeen  time.Time `db:"last_seen" json:"last_seen"`
    Mode      string    `db:"mode" json:"mode"` // p2p | relay | saas
}
```

### 3.2 Tipos TypeScript del frontend (`frontend/src/types.ts`)

```ts
export interface Alumno {
  id: number; nombre: string; email: string; escuela: string; created_at: string;
}
export interface Material {
  id: number; titulo: string; tipo: "libro" | "doc" | "video";
  url: string; autor_id: number; creado_en: string;
}
export interface Sandbox {
  id: number; nombre: string; lenguaje: "python" | "node" | "go";
  repo_url: string; autor_id: number; sala_id: number;
  visibility: "private" | "federated";
  modo: "job" | "service";
  servicio?: string; // modo service: puerto+ruta/url asignada
  retention: "historical" | "ephemeral"; // historical por defecto; ephemeral si el alumno se niega a guardar
  created_at: string;
}
export type RunStatus = "pending" | "running" | "succeeded" | "failed" | "cleaned";
export interface ContainerRun {
  id: number; sandbox_id: number; backend: "docker" | "firecracker" | "unikernel";
  status: RunStatus; ext_id: string; started_at: string;
  finished_at?: string; exit_code?: number;
}
export interface FederationPeer {
  id: number; node_id: string; name: string; endpoint: string;
  last_seen: string; mode: "p2p" | "relay" | "saas";
}
```

---

## 4. API REST

Base: `https://<host>/api/v1`. Respuestas JSON. Auth: docente con OAuth de Google (§0.6);
alumno entra por código de sala (token de sesión efímero, §0.6). No hay login por alumno.

### 4.1 Materiales
| Método | Ruta | body | respuesta |
|--------|------|------|-----------|
| GET | `/materials` | — | `Material[]` |
| POST | `/materials` | `{titulo, tipo, url}` | `Material` |
| GET | `/materials/:id` | — | `Material` |

### 4.2 Sandboxes (lo que antes llamábamos "proyectos")
| Método | Ruta | body | respuesta |
|--------|------|------|-----------|
| GET | `/sandboxes` | — | `Sandbox[]` |
| POST | `/sandboxes` | `{nombre, lenguaje, repo_url, sala_id, visibility, modo:"job"|"service", retention?:"historical"|"ephemeral"}` | `Sandbox` |
| GET | `/sandboxes/:id` | — | `Sandbox` (registro; ve historial, logs, artifact) |
| POST | `/sandboxes/:id/instantiate` | `{backend?:"docker", modo?:"job"|"service"}` | `ContainerRun` (re-lanza desde el registro) |

### 4.3 Ejecuciones (Runner)
| Método | Ruta | body | respuesta |
|--------|------|------|-----------|
| POST | `/sandboxes/:id/run` | `{backend?:"docker", modo?:"job"|"service"}` | `ContainerRun` (status: pending) |
| GET | `/runs/:id` | — | `ContainerRun` (estado actual) |
| GET | `/runs/:id/logs` | — | `text/event-stream` (SSE: stdout/stderr en vivo) |
| POST | `/runs/:id/stop` | — | `ContainerRun` (status: failed/cleaned) |

Ejemplo de `POST /sandboxes/42/run` (modo service, ej. servidor PHP):
```json
{ "backend": "docker", "modo": "service" }
```
Respuesta:
```json
{ "id": 7, "sandbox_id": 42, "backend": "docker", "status": "running",
  "ext_id": "abc123", "servicio": "https://ocicat.edu/s/42", "started_at": "2026-08-20T12:00:00Z" }
```

### 4.4 Federación
| Método | Ruta | body | respuesta |
|--------|------|------|-----------|
| GET | `/federation/peers` | — | `FederationPeer[]` |
| POST | `/federation/peers` | `{node_id, name, endpoint, mode}` | `FederationPeer` |
| POST | `/federation/announce` | `ProjectAnnounce` (ver §4.2) | `202 Accepted` |
| GET | `/federation/sync` | `?since=ts` | `ProjectAnnounce[]` |

---

## 2. Runner — aislamiento, límites, eventos y rate limiting

El Runner es la capa que ejecuta proyectos de alumnos en sandbox. Su responsabilidad:
aislar, limitar recursos, emitir eventos, y **frenar abusos antes de tocar Docker**.

### 2.1 Interfaz `SandboxBackend` (abstracción)
Toda implementación (Docker hoy, Firecracker y unikernel después) cumple la misma interfaz.
La API nunca sabe qué backend corre. Cambiar de Docker a Firecracker = cambiar una línea de
config, no reescribir handlers.

```go
// runner/backend.go
type SandboxBackend interface {
    // Build imagen a partir del repo del sandbox (cachea por hash).
    Build(ctx context.Context, s *store.Sandbox) (imageRef string, err error)
    // Run lanza el sandbox con límites. Devuelve un handle para streams y stop.
    Run(ctx context.Context, imageRef string, lim Limits) (Handle, error)
    // Stop mata el sandbox y libera recursos.
    Stop(ctx context.Context, id string) error
    // Logs devuelve un stream de stdout/stderr del sandbox.
    Logs(ctx context.Context, id string) (<-chan LogChunk, error)
    // Kind identifica el backend ("docker" | "firecracker" | "unikernel").
    Kind() string
}

type Handle interface {
    ID() string
    Wait() (exitCode int, err error)
}
```

### 2.2 Struct de límites (`runner/limits.go`)
Un solo struct parametriza el aislamiento. El backend Docker lo mapea a flags; Firecracker
a los equivalentes de micro-VM.

```go
// runner/limits.go
type Limits struct {
    CPUQuotaUSD   int    // % de un core, ej. 100 = 1 core. Docker: --cpu-quota/--cpu-period.
    MemoryMB      int    // Docker: --memory. Firecracker: vCPU mem.
    PidsMax       int    // Docker: --pids-limit. Frena fork bombs.
    DiskMB        int    // Docker: --storage-opt size=. FS efímero.
    NetworkMode   string // "none" (MVP) | "bridge" (Fase 3 si hace falta egress permitido).
    TimeoutSec    int    // tiempo máximo de ejecución; tras esto Stop().
    Privileged    bool   // SIEMPRE false en Ocicat.
    ReadOnlyFS    bool   // true: el contenedor no escribe al host.
}
```

Defaults MVP por proyecto (se pueden sobreescribir por institución):
`CPUQuotaUSD=100, MemoryMB=128, PidsMax=128, DiskMB=256, NetworkMode="none", TimeoutSec=120`.

### 2.3 Máquina de estados (`ContainerRun.Status`)
```text
pending ──run solicitado──> running ──éxito──> succeeded
   │                           │
   │                        ┌──error/timeout──> failed ──cleanup──> cleaned
   └──rechazado por rate limit──> rejected (no se crea sandbox)
```
- **Ciclo del CONTENEDOR vs del REGISTRO:** `cleaned` libera el contenedor/recursos, pero el
  *registro* del sandbox persiste según `Sandbox.Retention`:
  - `historical` (por defecto): el sandbox queda como **registro histórico re-accedible y
    re-instanciable** — la "muestra histórica" de todos los proyectos, habidos y por haber.
    Cualquiera (invitado, alumno de otra escuela, el autor) puede:
      * **Acceder/ver**: `GET /sandboxes/:id` muestra metadata, logs y artifact guardado, sin
        ejecutar.
      * **Instanciar de vuelta**: `POST /sandboxes/:id/instantiate` relanza el sandbox desde el
        registro (re-build desde repo/commit o reusar artifact). Modo `job` corre de nuevo; modo
        `service` levanta la URL otra vez.
    El contenedor murió; el registro vive y es reejecutable. Esto es el puente de la federación:
    un alumno de otra provincia no solo *ve* el proyecto ajeno, sino lo *corre* en su instancia.
  - `ephemeral`: el alumno se negó a guardar → el registro se borra al limpiar el contenedor.
- **Modo service:** en vez de `succeeded/failed`, el contenedor queda `running` y se le asigna
  una URL/ruta (ej. `https://<host>/s/<id>`). Vive hasta `POST /runs/:id/stop` o expiración.
  El RateLimiter lo cuenta como sandbox vivo igual que un job.
La API expone el estado vía `GET /runs/:id` y los eventos vía SSE (`GET /runs/:id/logs`).

### 2.4 EventBus y RATE LIMITING (decisión del usuario)
Todo pasa por un bus de eventos en-process (canal de Go). El RateLimiter se sienta EN el bus,
antes del backend, y frená abusos de "playground" limitando (a) eventos procesados y (b) recursos.

```go
// runner/events.go
type EventBus struct {
    ch      chan Event
    rl      *RateLimiter
    backend SandboxBackend
}

type Event struct {
    Type     string // "run.requested" | "run.started" | "run.log" | "run.finished" | "run.cleaned"
    RunID    int64
    SchoolID int64  // para presupuesto por institución
    Chunk    LogChunk
}

// RateLimiter: dos capas.
type RateLimiter struct {
    // Capa A: eventos. Cuántas ejecuciones por ventana por escuela.
    perSchool tokenBucket // ej. 5 runs/min por escuela
    // Capa B: recursos. Cuántos sandboxes concurrentes y cuánto recurso total por escuela.
    concurrency map[int64]int   // schoolID -> sandboxes vivos
    maxConcurrent int          // ej. 4 por escuela en server viejo
    totalMemMB  map[int64]int   // presupuesto de RAM usada por escuela
    maxMemMB    int            // ej. 512 MB por escuela
}
```

Algoritmo de admisión (en `EventBus.Dispatch`):
```
al recibir run.requested(schoolID):
  if !perSchool.allow(schoolID):        -> emitir run.rejected "too many requests, esperá"
  if concurrency[schoolID] >= maxConcurrent: -> run.rejected "límite de ejecuciones simultáneas"
  if totalMemMB[schoolID] + Limits.MemoryMB > maxMemMB: -> run.rejected "sin memoria disponible"
  si pasa: concurrency[schoolID]++; totalMemMB[schoolID]+=Limits.MemoryMB
           emitir run.started -> backend.Run(...)
al recibir run.finished / run.cleaned(schoolID):
  concurrency[schoolID]--; totalMemMB[schoolID]-=Limits.MemoryMB
```

Consecuencia: un alumno que spammea "▶ Ejecutar" no tumba el server viejo de la escuela.
El límite se aplica por evento y por recurso, en el único punto donde pasa todo. Sin proxy extra.
**Nota (definido con el usuario):** el `maxConcurrent` es la cantidad de sandboxes **PRENDIDOS**
(contenedor vivo) en un momento dado. Los sandboxes históricos duermen como registro y no consumen
presupuesto; al instanciarse (§2.3/`instantiate`) entran al RateLimiter y, si hay lugar, se
"prenden"; si no, esperan o se rechazan. Así no todos los sandboxes viven a la vez.

### 2.5 Algoritmo `Launch(sandbox)` paso a paso
```
1. Validar sandbox (lenguaje soportado, repo accesible).
2. Emitir run.requested -> RateLimiter admite o rechaza (§2.4). Si rechaza: fin.
3. Build imagen:
   - si imagen para (repoURL, commit) ya está en cache -> reusar (dedupe, ver Parte 6).
   - si no -> docker build desde plantilla docker/<lenguaje>/Dockerfile + repo del alumno.
4. Definir Limits (§2.2) según modo:
   - modo "job": net=none, timeout 120s, FS efímero (playground efímero).
   - modo "service": net=bridge CON ingress solo a la ruta asignada (ej. /s/<id>),
     SIN egress salvo que la sala lo habilite; sin timeout (vive hasta stop/expiración);
     el puerto interno se mapea a la ruta del proxy inverso.
5. Crear sandbox con Limits: sin privilegios, mem/CPU/pids fijos, FS efímero.
6. Emitir run.started.
7. Arrancar contenedor en detach.
   - modo "service": registrar ruta/url en el proxy inverso; el sandbox queda accesible.
8. Stream de logs: backend.Logs() -> canal -> emitir run.log por SSE (modo job y service).
9. Watchdog:
   - modo "job": si TimeoutSec se cumple -> backend.Stop() -> run.finished(exit=124 "timeout").
   - modo "service": no hay timeout; vive hasta stop o TTL de la sala.
10. Cleanup (modo job al terminar / service al stop):
    backend.Stop() + rm contenedor + liberar contador de RateLimiter (§2.4).
11. Retención (§2.3): si Sandbox.Retention == "historical" -> el registro persiste (histórico);
    si == "ephemeral" -> borrar el registro también.
12. Emitir run.cleaned. Fin.
```

### 2.6 Por qué esto sirve al target
- Server viejo: `maxConcurrent=4`, `maxMemMB=512` evitan que 30 alumnos a la vez lo maten.
- Personal no experto: no configura nada; los defaults están en el binario.
- Cambiar a Firecracker (Fase 3): solo se implementa `SandboxBackend` nuevo; el RateLimiter,
  la máquina de estados y la API no cambian.

---

## 3. Frontend — framework, páginas y dos modos de ayudar a la PC mal preparada

### 3.1 Problema real de las escuelas (contexto del usuario)
Las PCs de las escuelas no están 100% preparadas: el alumno llega a una máquina que no tiene
las herramientas que necesita. Ocicat resuelve eso de dos formas:

- **Modo A — instaladores en la plataforma:** todos los ejecutables/instaladores de las
  herramientas de la materia (Python, VSCode, compiladores) viven como "materiales"
  descargables. El alumno entra desde la PC despreparada y se baja lo que necesita. Es barato:
  reusa el módulo de materiales. Feature del MVP.
- **Modo B (descartado):** VM de Windows personal por alumno. Se descarta: corre en el server
  viejo de la escuela (no da), podrían instanciarse 300 y hay que mantenerlas, y las PCs de los
  alumnos dejarían de servir (serían solo display del contenedor). Un modelo "host-líder como
  intermediario" tampoco se mete al core: un TeamViewer ya lo resuelve.

**Alternativa que SÍ se adopta — sandbox efímero de "Equipo"/clase:** un contenedor compartido
por toda la clase donde todos escriben/editan el mismo documento o proyecto, sin pasarse archivos
ni pelear dependencias. Es el mismo `SandboxBackend` (§2.1) pero multi-usuario y con workspace
compartido. Ahorra el tiempo de "te paso el archivo / a mí no me anda / no sé prepararlo". Se
anota como feature del Runner (modo "team sandbox"), Fase 2.

### 3.2 Comparación de frameworks y librería de componentes
| Framework | SSG/SSR | Peso cliente | Fit para target (PC viejo) |
|-----------|--------|--------------|----------------------------|
| **Astro** | SSG (islas) | **Mínimo** | **Ideal**: no come RAM, va a Cloudflare |
| Nuxt | SSR/SSG | Medio | Bien, más pesado |
| Vue SPA | CSR | Alto | Malo para PC viejo |

**Framework: Astro** (decisión del usuario). **Librería de componentes: se busca ligera y
moderna, no Shadcn (copy-paste en React/Tailwind, más pesada en build).** Alternativas ligeras:
- **PicoCSS** — solo CSS, cero JS, look moderno, ligereza máxima. Ideal para Astro estático.
- **BeerCSS** — Material 3, ligero, muy moderno.
- **daisyUI** (sobre Tailwind) — componentes listos, liviano usando solo lo necesario.
- *Shadcn* queda como opción si se quiere React/Tailwind, pero no es la recomendada para el target.

**Recomendación: Astro + PicoCSS (o daisyUI).** UI moderna, casi nulo JS en cliente, anda en PC
viejo, se sube a Cloudflare. Las partes interactivas (logs en vivo, wizard) son islas mínimas.

### 3.3 Páginas (routes del frontend)
- `/` — landing institucional (afordance Drive/Classroom).
- `/materials` — catálogo de documentos/imágenes/dibujos/videos + **instaladores** (Modo A).
- `/sandboxes` — lista de sandboxes; tarjeta con "▶ Ejecutar".
- `/sandboxes/:id` — detalle + botón ejecutar + **logs en vivo vía SSE**.
- `/run/:id` — viewer de ejecución (streaming de `run.log` por SSE).
- `/federation` — panel de peers conectados (post-MVP).
- `/setup` — wizard de onboarding (solo primera vez; ver §0.1).

### 3.4 Consumo de API
El frontend usa `src/lib/ocicat-api.ts` (tipos de `types.ts`). Para logs en vivo:
`EventSource('/api/v1/runs/:id/logs')` → el backend emite `run.log` por SSE. Las demás
llamadas son `fetch` JSON normal. El frontend NUNCA toca Docker ni la DB.

---

## 4. Federación — protocolo, descubrimiento y replicación

La federación es lo que convierte Ocicat de "un repo por escuela" en "una comunidad de escuelas".
Dos planos: (a) **metadata de sandboxes** (qué hay, dónde) y (b) **datos** (el contenido
compartido). Turso (§0.5) simplifica (b); el protocolo de abajo cubre (a) y el descubrimiento.

### 4.1 Modelo de identidad
Cada instancia Ocicat tiene un `node_id` (UUID). Cada sala y cada sandbox llevan el `node_id`
de su origen. Un sandbox federado = `{node_id, sandbox_id_local}` resuelto contra el peer origen.
**Trazabilidad del autor (definido con el usuario):** `Sandbox.AutorID` apunta al `Alumno` local
de la sala (token de sesión efímero, §0.6). Con nombre/email opcional, se puede buscar "proyectos
de Juan" y suspender el token si abusa. Sin nombre, queda como alias anónimo de sala. No es
identidad global: un mismo pibe en otra sala es otro registro local. No hay bloqueo por IP/dispositivo.

### 4.2 Mensajes del protocolo (JSON sobre HTTPS)
| Tipo | Dirección | cuerpo | efecto |
|------|-----------|--------|--------|
| `announce` | local → peers | `ProjectAnnounce` | da a conocer un sandbox nuevo/actualizado |
| `query` | local → peer | `{node_id?, filtro}` | pide lista de sandboxes |
| `sync` | peer → local | `ProjectAnnounce[]` | responde con metadatos desde `since` |
| `ping` | local ↔ peer | `{}` | latido; actualiza `last_seen` del peer |
| `fetch` | local → peer | `{node_id, sandbox_id}` | pide el contenido/binario del sandbox (bajo demanda) |

`ProjectAnnounce` (metadato, NO el binario):
```json
{
  "node_id": "uuid-instancia",
  "sandbox_id": 42,
  "nombre": "Juego en Python",
  "lenguaje": "python",
  "autor": "Escuela Técnica 3",
  "visibility": "federated",
  "modo": "job",
  "retention": "historical",
  "version": 3,
  "updated_at": "2026-08-20T12:00:00Z",
  "checksum": "sha256:ab12..."
}
```

### 4.3 Descubrimiento: relay (índice) vs P2P puro
- **Relay como índice (modo A, recomendado MVP):** el relay SOLO mantiene la lista de peers
  (`{node_id, endpoint}`) — no guarda datos ni corre Docker. Esto cabe en **Cloudflare Workers +
  KV/D1** (índice de peers de escuelas entra en el free tier de Workers; escala solo, sin VPS que
  mantener). El VPS solo haría falta para la instancia hosteada (binario con Docker, que no corre
  en Workers) — y eso es el modo servicio, opcional. Un nodo nuevo pregunta al relay "¿quién hay?"
  y luego habla P2P con cada peer. El relay puede ser el del autor (Cloudflare) o uno comunitario.
  Sigue siendo FOSS: el índice es solo descubrimiento.
- **P2P puro (modo B, post-MVP):** DHT ligera entre instancias. Más resiliente, más complejo.
  Se deja para después; el protocolo de mensajes es el mismo, cambia quién resuelve los peers.

### 4.4 Replicación de DATOS con Turso (clave)
Como usamos Turso/LibSQL (§0.5), la replicación de la metadata entre instancias NO es un protocolo
que inventamos: es **replicación nativa de LibSQL**. Cada instancia puede configurar su DB para
replicar la tabla `sandboxes` (solo metadatos, `visibility: federated`) hacia los peers autorizados.
El `sync` de §4.2 queda como fallback/manual; la vía feliz es replicación de DB. Los binarios de
los sandboxes (imágenes Docker) NO se replican por defecto: se resuelven bajo demanda con `fetch`
(§4.2) contra el nodo origen, o se reconstruyen desde el repo del alumno. El `fetch` es justo lo
que permite **re-instanciar un registro histórico remoto** (§2.3): un nodo pide el artifact/binario
del sandbox federado y lo relanza en su propia instancia. Así la "muestra histórica" es reejecutable
entre escuelas, no solo visible.

### 4.5 Reconciliación (MERGE) de conflictos
Dos instancias editan el mismo sandbox federado (raro, pero posible en modo team). Estrategia:
- Cada `ProjectAnnounce` lleva `version` (vector de versión por `node_id`) y `updated_at`.
- Al recibir un announce con `version` mayor → se acepta. Con `version` menor → se ignora.
- Igual `version` + distinto `checksum` → se conservan ambos como `sandbox_id:conflict-N` y un
  humano (docente) elige. No auto-sobrescribir.
- Esto es last-write-wins por version, sin bifurcar la red.

### 4.6 Relay vs instancia hosteada (aclaración del usuario)
Relay (descubrimiento) e instancia hosteada por el autor (§al final del doc) son **sistemas
separados** que pueden verse bajo una misma URL. El relay solo indexa peers; la instancia
hosteada corre el binario FOSS por el usuario que no quiere mantener el suyo.

---

## 5. Seguridad — matriz de amenazas y contramedidas

El riesgo central de Ocicat es que ejecuta código ajeno (proyectos de alumnos) en el server de la
escuela. La seguridad no es un feature: es el producto. Abajo, amenazas por fase y mitigación.

### 5.1 Matriz de amenazas
| # | Amenaza | Vector | Impacto | Mitigación (Fase 1) | Mitigación (Fase 3) |
|---|---------|--------|---------|---------------------|---------------------|
| A1 | **Escape de sandbox** | bug de Docker / config errónea | compromete el host de la escuela | sin privilegios, `network=none`, FS efímero, `read-only` | Firecracker (micro-VM) / unikernel |
| A2 | **DoS por ejecuciones** | alumno spammea "▶ Ejecutar" | server viejo colapsa | RateLimiter §2.4 (concurrencia+RAM+eventos) | igual + cuotas por sala más finas |
| A3 | **Exfiltración de datos** | proyecto malicioso intenta red saliente | fuga de datos de la escuela | `network=none` por defecto | egress allowlist opcional por sala |
| A4 | **Fork bomb / PID explosion** | `:(){ :|:& };:` en el sandbox | host sin PIDs | `pids-limit` en Limits §2.2 | igual en micro-VM |
| A5 | **Abuso de uploads (spam)** | suben archivos gigantes | llenan disco de la escuela | límite de tamaño + cuota por sala | dedupe + compresión |
| A6 | **Escalada en federación** | peer malicioso anuncia metadatos falsos | desinformación en catálogo | firma opcional de `announce` por node_id | Web of Trust entre instancias |
| A7 | **Abuso de sala** | alumno abusivo en la sala | molesta/flood | rotación de código + vetar nombre/email §0.6 | baneo por reputación de instancia |
| A8 | **Takeover de la instancia** | alguien accede al server | control total | OAuth Google solo docente + token de sala | 2FA docente + audit log |

### 5.2 Principios de defensa
- **Defensa en profundidad:** cada capa asume que la anterior puede fallar. Si Docker escapa, el
  host ya no corre nada crítico y el FS es efímero.
- **Default deny:** sin red, sin privilegios, sin escritura persistente salvo que se habilite.
- **Aislamiento por escuela/sala:** el presupuesto del RateLimiter (§2.4) acota el blast radius a
  una sala, no a todo el server.
- **Sin identidad global del alumno:** si un token se filtra, solo afecta esa sala efímera.

### 5.3 Por qué Fase 1 es aceptable
Docker estándar con las mitigaciones de A1–A5 es suficiente para el MVP en contexto escolar: el
atacante (un alumno) tiene incentivo bajo y la superficie está acotada. Firecracker/unikernel
(Fase 3) entran cuando se suma la instancia hosteada (donde el riesgo es de terceros desconocidos,
no del aula). No se adelanta por sobre-ingeniería.

---

## 6. Algoritmos — presupuesto, dedupe y MERGE

Tres algoritmos que sostienen el sistema en el server viejo de la escuela y en la federación.
Todos son deterministas y baratos (corren en el binario único, sin servicios extra).

### 6.1 Presupuesto de contenedores por sala/escuela (del RateLimiter §2.4)
Objetivo: nunca sobrepasar los recursos del server viejo, repartiendo justo por sala.

```
estado por escuela E:
  concurrentes[E] = 0
  ramUsada[E]    = 0
  maxConcurrent[E] = 4        // tunable por escuela
  maxRamMB[E]     = 512       // tunable por escuela

al solicitar run(sala S, escuela E, lim Limits):
  if concurrentes[E] >= maxConcurrent[E]:
      rechazar("límite de ejecuciones simultáneas")
  if ramUsada[E] + lim.MemoryMB > maxRamMB[E]:
      rechazar("sin memoria disponible")
  if !tokenBucket[E].allow():            // 5 runs/min por escuela
      rechazar("demasiadas ejecuciones, esperá")
  // admite
  concurrentes[E] += 1
  ramUsada[E]    += lim.MemoryMB
  ventana[S].insert(runID)               // para cuota por sala
  emitir run.started

al terminar/cleaned(run, S, E, lim):
  concurrentes[E] -= 1
  ramUsada[E]    -= lim.MemoryMB
  ventana[S].remove(runID)
```
Nota: el bucket por escuela evita que UNA sala con 30 alumnos mate a otra sala del mismo server.

### 6.2 Dedupe de imágenes de sandbox
Objetivo: no rebuildar ni re-descargar la misma imagen N veces (disco y red de la escuela).

```
clave(repourl, commit, lenguaje) -> hash = sha256(repourl|commit|lenguaje)

al Build(s Sandbox):
  h = clave(s.RepoURL, s.Commit, s.Lenguaje)
  if cacheImagen[h] existe Y no expirada:
      return cacheImagen[h]            // hit: reusa
  img = dockerBuild(plantilla[lenguaje], repo(s))
  cacheImagen[h] = img
  // GC: si cacheImagen.size > maxCacheMB, borrar las menos usadas (LFU)
  return img
```
Beneficio: 30 alumnos con el mismo repo de clase → 1 sola build. El `commit` en la clave evita
servir código viejo tras un push del alumno.

### 6.3 MERGE de federación (reconciliación de metadata)
Objetivo: dos instancias editan el mismo sandbox federado sin bifurcar la red. (Base en §4.5.)

```
al recibir announce(a: ProjectAnnounce) desde peer P:
  local = tabla_sandboxes[a.node_id, a.sandbox_id]
  if local == nil:
      insertar(a); emitir a subscribers; return
  if a.version > local.version:
      // último escritor gana por version
      local = a; emitir; return
  if a.version < local.version:
      ignorar; return
  if a.version == local.version AND a.checksum != local.checksum:
      // conflicto real: conservar ambos, no sobrescribir
      guardar como conflict-N = a
      notificar al docente dueño de la sala para que elija
      return

al pedir sync(desde=ts) a peer P:
  return [a in tabla_sandboxes where a.updated_at > ts AND visibility=="federated"]
```
Con Turso (§4.4) la vía feliz es replicación nativa de la tabla; este MERGE es el fallback para
pares que no replican DB o para conflictos que la replicación no resuelve (mismo version distinto
checksum). La regla es determinista: version mayor gana, empate → conflicto manual, nunca auto-borra.

---

## 7. Roadmap y decisiones abiertas

### 7.1 Fases
| Fase | Alcance | Backend | Frontend | Fed |
|------|---------|---------|----------|-----|
| **MVP** | Materiales + sandboxes (job+service) + registro histórico + auth por sala + rate limiter + setup wizard | Go + Turso + Docker | Astro + daisyUI | relay en Cloudflare (índice) |
| **Fase 2** | Sandbox de equipo/clase (multi-usuario) + instanciar registros remotos + búsqueda por nombre | igual | igual + vista "Explorar" | replicación Turso entre pares |
| **Fase 3** | Firecracker (micro-VM) + unikernel opcional + instancia hosteada (modo servicio) + 2FA docente | SandboxBackend nuevo | igual | Web of Trust entre instancias |

### 7.2 Timeline sugerido
- MVP: lo que el DESIGN.md ya cubre (Partes 0–6). Es lo que se construye primero.
- Fase 2: una vez que el MVP corra en 1–2 escuelas reales y se valide el modelo de sala.
- Fase 3: cuando haya instancias hosteadas expuestas a terceros desconocidos (ahí sí importa
  endurecer el aislamiento más allá de Docker).

### 7.3 Decisiones abiertas (CERRADAS por el usuario)
1. **Unikernel runtime:** **Unikraft** (Fase 3).
2. **OAuth del docente:** **solo Google** (§0.6). Otros proveedores (Microsoft/O365) se suman
   después solo si amerita.
3. **Seats por defecto:** código abierto por defecto (§0.6), PERO el profe PUEDE forzar seats
   limitados (ej. 33 alumnos exactos, ni uno más). El cap de seats es una opción del docente, no
   del sistema.
4. **Artifact del registro:** no excluyentes. Se obtiene el repo y se cachea la imagen bajo
   demanda (§6.2); si ya existe la imagen se reusa (la imagen se crea sí o sí), si está
   desactualizada se elimina y se recrea. Dedupe + rebuild-on-change confirmado.
5. **TTL de sandboxes service:** **configurable, default 8 horas** sin `stop` → auto-apagado.

### 7.4 Cierre
El diseño quedó definido de punta a punta: infraestructura (binario Go + Turso + Cloudflare edge),
producto (fuente de verdad de contenido, affordances familiares), arquitectura (monolito modular
en capas + eventos + plugins), Runner (aislamiento + rate limiter + modos job/service + registro
histórico re-instanciable), frontend (Astro + daisyUI), federación (relay Cloudflare + replicación
Turso + MERGE), seguridad (matriz A1–A8), y algoritmos (presupuesto, dedupe, MERGE). Siguiente paso:
diseño visual con `nexu-io/open-design` (§0.7) y luego implementación del MVP.

---

## 9. Entregas y Wizard (definido con el usuario, 2026-08-21)

### 9.1 Consignas (lado docente)
El docente crea **Consignas** dentro de su sala:
- Título, instrucciones (texto), adjuntos opcionales (plantillas).
- Runtime esperado (Python, Web, Arduino...), fecha límite opcional.
- **Config de entrega (modificable en cualquier momento, incluso con entregas existentes):**
  - `intentos`: ilimitados (default) | N | uno solo.
  - `tardias`: permitidas (marcadas como tardías) | cerradas al vencimiento.

### 9.2 Wizard del alumno (4 pasos)
1. **Elegir consigna** — lista de tareas activas de su(s) sala(s) con vencimientos.
2. **Subir archivos** — drag & drop (.zip, .py, index.html...). Sin editor embebido
   (decisión: no vale la pena por ahora). Preview inline de lo subido.
3. **Probar** — corre el código en un sandbox efímero (modo job desechable) y ve el
   output antes de entregar. Reusa el runner del §2.
4. **Entregar** — snapshot del código + resultado de la prueba. El docente ve la
   entrega con estado (entregada / probada OK / error / tardía).

### 9.3 Previews de materiales y sandboxes
- **Materiales** (PDF/Docs/Videos/Imágenes): preview **inline** en la card
  (img/video/iframe PDF nativo; docs office = card con metadata + abrir).
  Click → página propia del documento (`/materials/:id`) para verlo directo
  en el navegador full-width + botón descargar.
- **Sandboxes:** modo service Web = iframe al puerto del contenedor;
  consola = terminal de logs (§3); Arduino = salida serial simulada (sin preview visual).

---

---

## Decisiones abiertas (tipadas)
1. **Frontend:** ¿Astro, Nuxt o Vue+Shadcn? (ver Parte 3)
2. **Auth:** ¿bearer token simple (Fase 1) o OIDC/SSO escolar (Fase 3)?
3. **Unikernel:** ¿qué runtime? (rumprun, Nanos/Unikraft) — Fase 3.

## Aclaración de modelo de despliegue (definido por el usuario)
Ocicat Bella es **FOSS**: tenés el código libre y lo hosteás vos. Además existe
**opcionalmente una instancia hosteada por el autor** (mismo binario, corriendo en
su server) por comodidad — sin obligación de usarla. No es "FOSS vs SaaS": es el
mismo producto, y cambia solo quién lo corre.

- **Relay y SaaS son sistemas separados**, aunque pueden exponerse bajo una misma URL.
- **Relay** = mecanismo de descubrimiento P2P entre instancias auto-hosteadas.
- **Instancia hosteada** = el binario FOSS corriendo donde el autor; el usuario no mantiene nada.

### Idea de federación estilo F-Droid (POST-MVP, pendiente de definir)
En vez de P2P puro, un modelo de **repositorios**: vos agregás un repo que trae un
set de endpoints/IPs para conectarse (como los repos de F-Droid traen listas de apps).
El relay sería ese índice; los datos de proyectos siguen en cada escuela.
**No entra al MVP** — el usuario lo dejó para pensar más adelante. Se registra acá
para no perderlo.
```

---

## 10. Templates de entorno y Dockerfiles (definido con el usuario, 2026-08-21)

### 10.1 El problema
El alumno no debería tener que definir comando de inicio, imagen base ni Dockerfile
para el caso común — pero sí poder hacerlo en casos avanzados, sin riesgos.

### 10.2 Templates de Docker (catálogo)
- Set predefinido por instancia: bun/react, C++/sqlite, python/numpy, wordpress,
  node/express, etc. Cada template define: imagen (allowlist), `start command`
  default (editable por quien lanza), modo (job/service), puertos expuestos.
- El 90% de los alumnos solo **elige un template** y corre.

### 10.3 Composer visual de herramientas
- UI con catálogo de herramientas (logo + nombre): WordPress, C++, SQLite, Python,
  Bun, React... El alumno "arma" su template seleccionando herramientas; la
  plataforma compone la imagen (capas pre-construidas por herramienta).
- Las combinaciones resultantes se cachean como imágenes derivadas (dedupe §6).

### 10.4 Dockerfile propio (alumno avanzado) — con rejas
- **Lint previo obligatorio**: prohibido `--privileged`, mounts del host,
  `COPY /etc/*`, red host, `ADD` de URLs externas; `USER` no-root final forzado.
- **Build aislado**: builder sin acceso al socket de Docker ni FS del host
  (BuildKit sandboxed; contenedores resultantes corren bajo gVisor/Kata si aplica).
- **Imágenes base**: solo allowlist cerrada, pull by digest desde registry espejo.
- Límites duros de CPU/RAM/disco/procesos también para el build; timeout de build.
- La imagen resultante vive en el registry local de la instancia.

### 10.5 Permisos del docente (por sala)
- Crear templates propios de la sala.
- Restringir: catálogo de herramientas visible, templates permitidos,
  y toggle **"Dockerfile propio"** (default OFF en salas nuevas).

---

## 11. Identidad del alumno — revisión (definido con el usuario, 2026-08-21)

### 11.1 El problema
El alias anónimo (`ALUMNO-12489`) no permite asignar notas, ni historial entre
clases, ni recuperar acceso tras perder el dispositivo. La identidad es requisito,
no optativo.

### 11.2 Modelo
- **Cuenta de alumno real**: nombre + email + credencial (password o magic link — a definir).
  OAuth Google opcional como alternativa, nunca obligatoria.
- La cuenta pertenece al alumno, no a la sala: una cuenta → N salas.
- **Unirse a sala**: el código de sala pasa a ser *invitación*, no identidad.
  Flujo /unirse: sin cuenta → mini-registro → código; con cuenta → login → código.
- Sesión server-side (token): entra desde cualquier dispositivo con su cuenta.
  Pérdida de dispositivo = login en el nuevo + "cerrar otras sesiones".
- El docente ve nombre real + email de sus alumnos. Alias/apodo visible opcional
  dentro de la sala (privacidad entre compañeros), trazabilidad interna por cuenta.
- **Reemplaza** la parte de §0.6 que decía "identidad opcional/anónima".

### 11.3 Preguntas abiertas que abre esto
- ¿Password o magic link? (magic link exige SMTP en cada instancia self-hosted)
- Datos de menores: consentimiento, qué se guarda, RGPD/normativa provincial.

---

## 12. Preguntas diferidas (para discutir más adelante)
- ¿Qué librería de OAuth usamos? (para docentes ya decidimos Google; falta elegir
  librería Go y si los alumnos pueden usar Google como opción de registro).
- ¿Usamos librerías para gestionar visualmente Docker (ej. Docker SDK/portainer-like)
  o lo hacemos manual con la API del engine? Impacta el composer visual de §10.3.

### 11.4 Ajustes de UX (feedback del usuario sobre mockups, 2026-08-21)
- **Alias/apodo por sala: eliminado.** Era ruido; la identidad es la cuenta.
- **Abandonar sala**: fuera del card (estaba junto a "Entrar", riesgo de click
  accidental). Pasa a menú de 3 puntos en el header del card → "Abandonar sala"
  con confirmación.
- **Modo visitante en login**: cualquiera puede entrar a la página y VER contenido
  educativo y proyectos, pero NO puede actuar como alumno (no entra a salas) sin
  cuenta registrada. Texto del login corregido: no se promete acceso directo con
  código sin registro.

---

## 13. Ajustes MVP post-research (definido con el usuario, 2026-08-21)

Fuente: doble análisis externo (UX Researcher + Product Manager) sobre los 11
mockups de la fase 2. Coincidencias y síntesis final.

### 13.1 Identidad del alumno — decisión final: magic link
- Registro/login del alumno: **nombre + email**. El nombre es identificación,
  el email recibe un **magic link** para iniciar sesión. Sin contraseñas.
- Resuelve ambas críticas: identidad persistente (notas, historial, multi-
  dispositivo) SIN el infierno de soporte de 30 contraseñas de alumnos.
- Docente: login propio (credencial única, puede ser password — es UN usuario).
- Recovery: reenvío de magic link; reset manual por el docente desde su panel.

### 13.2 Cambios al flujo alumno (aplicar a mockups)
- **Wizard de entrega: 4 → 3 pasos** (fusionar selector de template con subida
  de archivos). Checkpoint explícito: "¿Entregar? (intento 2 de 3)" → pantalla
  "Entregado ✓ a las HH:MM".
- **Sandbox de prueba**: accesible después de subir archivos, no paso obligatorio.
- **Estados intermedios visibles del sandbox**: encolando → descargando imagen →
  arrancando → listo. Feedback a <2s de cada acción (PCs viejas / red escolar).
- **Composer de Dockerfile → modo avanzado oculto**. Default: elegir lenguaje
  (Python/Web/C++/Arduino) + campo "paquetes extra" en texto plano.
- **Abandonar sala**: modal con consecuencia explícita ("vas a perder tus
  entregas de esta sala").

### 13.3 Cambios al flujo docente
- Labels en castellano docente: "entorno del ejercicio" (no Dockerfile),
  "qué pueden usar los alumnos" (no restricciones de templates).
- Rotar código: botón visible con confirmación + aviso "esto saca el acceso
  a toda la clase" (no escondido en menú).

### 13.4 Scope MVP recortado (audit PM)
- **Composer como pantalla dedicada: diferido a v2.** En su lugar, modo avanzado.
- Restricciones granulares por sala: diferidas (defaults sensatos hardcoded).
- Lint de Dockerfiles / allowlist UI: sin UI propia (interno backend).
- No-builds 2 meses: federación, OAuth/SSO/recovery, notificaciones/email
  transaccional (salvo magic link), modo service persistente, panel admin de
  escuela, PWA pulido, analytics elaborados, landing trabajada.
- Piloto mínimo: 1 escuela × 1 docente × 4 semanas. Éxito = retención docente
  semana 4 sin empuje + ≥60% alumnos entregando semanalmente.
- Riesgo #1: adopción docente. Mitigación: co-diseño previo con el docente real,
  onboarding cargado por el dev, plan B sin Docker (entrega de archivos funciona
  aunque fallen sandboxes).

### 13.5 Validación con usuarios (antes/durante implementación)
- 3 flujos a testear con papel/guerilla (5 usuarios c/u): join desde cero,
  wizard de entrega, crear sala+consigna (docentes, remoto).
- Métrica clave test join: % que entra sin ayuda.
- Landing: dos acciones arriba del fold — "Soy docente" / "Tengo un código".

### 13.6 Presupuesto de peso
- <300KB por página, webfonts no bloqueantes con fallback serif/sans del sistema.

### 13.7 Código global de escuela y rol Director (definido con el usuario, 2026-08-22)
- **Código global**: UNO por instancia. Da a invitados acceso de LECTURA a todos
  los proyectos y documentos públicos de la escuela. NUNCA consignas (material
  de evaluación, privado del curso). Ver sí, actuar no.
- **Rol Director (admin)**: encima de docente. Único que crea/gestiona/actualiza
  el código global. Crea y desactiva cuentas docente (el docente ya no se
  auto-registra: la puerta es el director). Jerarquía: director → docente → sala
  → alumno → invitado.
- **Panel admin (`admin.html`, pantalla nueva)**: código global (ver/regenerar),
  gestión de docentes, stats básicos de la instancia.
- **Landing**: fold mantiene 2 CTAs ("Soy docente" / "Tengo un código");
  "código de escuela" como link secundario. Plan B: un solo campo que detecta
  el tipo de código.
- **Nota de coherencia**: esto reemplaza el no-build "panel admin diferido a v2"
  de §13.4 — el panel admin BÁSICO (código global + docentes) entra al MVP;
  el panel admin completo sigue en v2.

### 13.8 Wizard de instalación (pantalla nueva `setup.html`)
Ya especificado en §0.1 pero nunca diseñado — ahora entra al MVP como pantalla:
- Flujo: bienvenida → nombre de la escuela → crear cuenta del DIRECTOR →
  código global generado (con hint "pegalo en el cartel del aula") → ir al panel.
- Es el punto donde NACE el director: sin setup no hay admin, sin admin no hay
  docentes, sin docentes no hay salas.

---

## 14. Equipo de agentes del proyecto (definido con el usuario, 2026-08-22)

Arquitectura de orquestación: **Hermes = Project Manager**. Despacha tareas a
instancias de agy/opencode/codex instruyendo el especialista de Agency Agents a
usar; los resultados vuelven al PM. El dev (Chaman) decide y commitea.

### 14.1 Roster fijo (7 + contingencia)
| # | Rol | Specialist Agency | Harness | Modelo | Convocatoria |
|---|-----|------------------|---------|--------|--------------|
| 1 | Frontend Dev | frontend-developer | agy | gemini 3.7 flash (medio) | Siempre activo |
| 2 | Backend Architect | engineering-backend-architect | opencode | x-f-preview-free | Siempre activo |
| 3 | QA/Test | test-automation-engineer | opencode | x-f-preview-free | Siempre activo |
| 4 | Code Reviewer | code-reviewer | codex | gpt 5.6 luna | Gate por commit |
| 5 | Security Auditor | security-ai-generated-code-auditor | codex | gpt 5.6 luna | Gate por módulo |
| 6 | DevOps Automator | devops-automator | agy | gemini 3.7 flash | Bajo demanda → piloto |
| 7 | UX Researcher | ux-researcher (+feedback-synthesizer) | agy | gemini 3.7 flash | Piloto, ciclo semanal |
| — | Contingencia | rapid-prototyper | agy | gemini 3.7 flash | Solo si métricas mal |

Consultor bajo demanda para arquitectura mayor (federación §4, Firecracker):
engineering-software-architect.

### 14.2 Retenido por humanos (no delegable)
Decisiones de producto/alcance, criterio pedagógico, revisión final y merges,
relación con la escuela y datos de menores, aceptación de riesgos de auditoría.
Regla: los agentes proponen artefactos; humanos deciden.

### 14.3 Ritual por fase
- **Fase 1 (Astro)**: PM descompone pantalla según DESIGN.md+CASOS-USO.md →
  Frontend Dev implementa (diff sin commitear) → QA testea → Code Reviewer
  revisa → Chaman/Hermes verifican contra mockup y commitean.
- **Fase 2 (Go)**: PM define contrato API (validado por Backend Architect en
  plan mode) → implementación → QA → review → gate Security por módulo cerrado
  (auth, sandboxes, entregas) obligatorio antes de mergear.
- **Fase 3 (Piloto)**: DevOps despliega (on-call), pasada final de seguridad
  pre-go-live, UX Researcher corre ciclo semanal métricas→síntesis→decisión humana.

### 14.4 Reglas de coordinación
- Un solo escritor por worktree; paralelismo solo con worktrees separados.
- Deliverable = artefacto verificable (diff, informe, suite verde). Exit 0 no es éxito.
- Review chain: implementador → tester → reviewer → (seguridad) → humano.

### 13.9 Navegación por rol (feedback del usuario sobre el frontend, 2026-08-22)

Pendientes detectados:
- Botón "ver material" y "crear material" no redirigen a nada (mockups sin
  destino implementado).

Nav por estado:
- **Sin registro (visitante anónimo)**: Admin (iniciar sesión) · Login/Registrarse ·
  Ingresar escuela con código.
- **Invitado (entró con código global)**: Materiales (todo) · Proyectos (todo).
- **Registrado (estudiante, profesor o admin)**: Perfil + logout · Salas ·
  Materiales (todo) · Proyectos (todo) · Consignas (pendientes).

### 13.9.1 Respuesta del UI Designer a la nav por rol (agy, 2026-08-22)
Veredictos sobre §13.9:
1. Nav unificada registrado: AJUSTAR — misma estructura base pero ítems/labels
   por rol (alumno "Mis Salas/Tareas", docente "Consignas"+crear en header de
   sección, director + "Gestión Escolar"). Creación vive en headers de sección,
   no como items globales.
2. "Salas" scope: alumno=sus salas; docente=las que dicta (+crear/administrar);
   admin=todas (vista global). Microcopy: preferir "Cursos"/"Aulas" sobre
   "Salas" (se confunde con Meet/chat).
3. Faltan: notificaciones (badge campana), panel Gestión Escolar (solo admin),
   indicador de escuela/ciclo lectivo. Sobra: "Consignas (pendientes)" como
   item — el filtro pendientes es vista default dentro de la página.
4. Admin separado del login: AJUSTAR URGENTE — unifica en 2 acciones anónimas:
   CTA "Ingresar con código de escuela" + link "Iniciar sesión". El login pide
   email y detecta rol: alumno→magic link; docente/director→pide password.
   Link discreto "Acceso institucional" para emergencias, nunca botón protagónico.

Riesgos UX flaggeados:
- Magic link en PCs compartidas de laboratorio: prever fallback QR/token temporal
  generable por el docente.
- Alto contraste en nav (monitores viejos de bajo brillo): nada de grises claros.

### 13.9.2 Decisiones finales de nav (usuario, 2026-08-22)
1. Crear material/consigna: DENTRO de la sala (el docente entra a la sala y usa
   el botón ahí). No como item global de nav. Coherente con el diseñador
   ("acciones de creación viven en headers de sección").
2. Login unificado: email → si rol no-alumno → transición a segunda página para
   password. Aprobado.
3. Microcopy: "Aulas" (no "Salas").
4. Notificaciones con badge: ENTRA al MVP.
5. Admin: ve todo y gestiona sin necesitar credenciales de docente (impersonación
   administrativa o acceso directo por rol, a definir en backend).
6. Magic link vs PCs compartidas: ABIERTA — evaluar alternativas antes de decidir
   volver a passwords. Investigación pendiente (QuickCard/QR de ClassLink como
   referencia del mercado escolar).

### 13.9.3 Login en PCs compartidas — decisión (usuario, 2026-08-22)
Descartado: QR/código por alumno impreso (escala mal: 850 alumnos / 3 aulas por
docente; imprimir es fricción).

Decisión: **login inverso por QR** (patrón WhatsApp Web):
- El alumno tiene una app/mini-app en su CELULAR (su dispositivo personal, no
  depende del laboratorio).
- En la PC del colegio, Ocicat muestra un QR efímero ("iniciar sesión").
- El alumno escanea el QR con su celular → confirma → la PC queda con SU sesión
  (temporal, se cierra sola al terminar o al cerrar el navegador).
- El magic link queda como método alternativo para quien tenga email accesible.
- Pendiente de diseño: forma de la "app" del celular (PWA vs web móvil simple),
  emparejamiento inicial del dispositivo con la cuenta, y expiración de sesión.

### 13.9.4 Cierre del login (usuario, 2026-08-22)
- App del alumno: **PWA** (liviana, <300KB como el resto).
- Emparejamiento: una vez vía magic link en el celular → después escaneo QR.
- Fallback sin celular: login completo desde la PC con email (propio o creado
  en el momento) + magic link.
- Auditoría de seguridad del flujo QR+magic link: EN CURSO (Security Auditor).

### 13.9.5 Auditoría de seguridad del login (Security Auditor, 2026-08-22)
VEREDICTO: APROBADO CON CONDICIONES — sin bloqueantes. 6 condiciones obligatorias
antes de producción:
1. Registro de alumnos CERRADO: alta sólo vía docente/admin (import/invitación),
   nunca self-service abierto. (Decisión de arquitectura más importante.)
2. Pairing inicial del celular SOLO vía magic link validado por email.
3. QR: un solo uso, TTL ≤90s, entropía ≥128 bits, URL del dominio oficial
   (nunca shortener), confirmación en celular mostrando identidad de la PC
   ("¿Conectar con LAB-PC07?"), regeneración ~60s.
4. Sesión de PC: TTL ≤60 min, inactividad ≤10 min con aviso, cookies
   HttpOnly/Secure/SameSite=Strict, cero persistencia.
5. Emails duplicados rechazados + límite de dispositivos por cuenta (~3).
6. Data minimization (nombre/curso/email, nada más) + página de privacidad
   simple en castellano + borrado real de cuenta.

Mitigaciones por vector: quishing→QR efímero+identidad PC; replay→single-use
TTL; hijacking→sesión corta no persistente; CSRF→SameSite+POST con body;
fuerza bruta→entropía+rate limit; doble claim→claim atómico.

Post-MVP (no bloqueante): re-confirmación celular periódica, panel "mis
dispositivos", rate limit global auth. Recovery de email perdido: reset manual
del admin, documentado.

Nota: emails escolares compartidos = riesgo real; preferir casillas individuales
del alumno, documentar como requisito de despliegue.
