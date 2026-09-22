# Ocicat Bella — Contrato API v1 (borrador)

**Estado:** borrador para revisión (Backend Architect, 2026-08-22). No implementado.
**Fuentes:** `CASOS-USO.md` (18 CUs), `DESIGN.md` §9–13 (especialmente §13.4 scope, §13.7 código global/director, **§13.9.5 — 6 condiciones de seguridad obligatorias**). Supersede el viejo boceto API de DESIGN.md §4, que es anterior al rol director, magic link y QR inverso.
**Stack:** Go + Turso + Docker runner, binario único self-hosted.

---

## 0. Convenciones globales

- Base: `https://<host>/api/v1/*`. Salvo `/setup/*` y los endpoints marcados públicos, todo requiere sesión.
- Formato: JSON UTF-8 (`Content-Type: application/json`). Uploads de archivos: `multipart/form-data`. Streams: SSE (`text/event-stream`).
- IDs: strings opacos (ULID). Timestamps: RFC3339 UTC.
- Errores uniformes en TODOS los endpoints:

```json
{ "error": { "code": "invalid_code", "message": "este código ya no funciona; pedile el nuevo a tu docente" } }
```

| HTTP | `error.code` | Cuándo |
|------|--------------|--------|
| 400 | `validation_error` | Body malformado, campo inválido (message detalla el campo) |
| 400 | `file_too_large` | Archivo > límite (10 MB por archivo) |
| 401 | `unauthenticated` | Sin sesión o sesión expirada |
| 403 | `forbidden` | Autenticado sin permiso sobre el recurso |
| 403 | `guest_read_only` | Invitado/anónimo intenta actuar (CU-1/CU-14) |
| 404 | `not_found` / `invalid_code` | Recurso inexistente; código de sala/escuela inválido o rotado |
| 409 | `email_already_exists` | Alta con email duplicado (§13.9.5 cond. 5) |
| 409 | `device_limit_reached` | Pairing superaría ~3 dispositivos (cond. 5) |
| 409 | `already_member` / `already_claimed` / `attempts_exhausted` / `deadline_passed` | Conflictos de estado |
| 400 | `invalid_token` / `invalid_credentials` | Token de magic link malformado; email o contraseña incorrectos |
| 403 | `account_disabled` | Cuenta desactivada por el director (§9.2, CU-15) |
| 409 | `setup_already_done` | Setup ya realizado (solo primera configuración de instancia, §1) |
| 409 | `attempt_conflict` | `confirm_attempt` no coincide con el intento real (cliente desactualizado) |
| 410 | `token_used` / `token_expired` | Magic link o QR ya usado / vencido |
| 429 | `rate_limited` | Rate limit (ver §1.4); incluye `retry_after` |
| 503 | `capacity_unavailable` | Runner sin presupuesto y cola dura llena (§4) |

### 0.1 Roles y jerarquía

`director → docente → aula → alumno → invitado(código global) → anónimo`

- **director** (admin): dueño de la instancia. Pasa todos los checks de docente (ve todo y gestiona sin credenciales de docente, §13.9.2).
- **docente**: credencial password. Crea aulas, consignas, materiales; da de alta alumnos.
- **alumno**: SIN password. Magic link + QR inverso.
- **invitado**: cookie de solo-lectura obtenida con el código global de escuela.
- **anónimo**: sin cookie; lectura de contenido público.

### 0.2 Sesiones

Cookie única `ocicat_session`: **HttpOnly, Secure, SameSite=Strict**, `Path=/`. Dos clases:

| Clase | Quién | Vida | Persistencia |
|-------|-------|------|--------------|
| `pwa` | alumno emparejado (celular) | persistente (30 días sliding), revocable | sí |
| `staff` | docente/director navegador | persistente (7 días sliding) | sí |
| `pc_temporal` | alumno en PC compartida (QR inverso CU-16 o magic link desde PC CU-2) | **TTL absoluto ≤60 min + inactividad ≤10 min con aviso previo** | **ninguna**: cookie sin `Max-Age`, nada en localStorage |

Reglas:
- Las sesiones `pc_temporal` NO consumen cupo del límite de dispositivos (~3, que cuenta solo pairings PWA).
- CSRF: `SameSite=Strict` cubre POST cross-site; adicionalmente el backend valida `Origin` en toda mutación (mitigación §13.9.5).
- Alumno en PC compartida SIEMPRE recibe clase `pc_temporal`, use QR o magic link. Solo la PWA emparejada (magic link validado por email en el celular, cond. 2) obtiene sesión persistente.
- Rotación de token de sesión en cada upgrade de privilegio (login → sesión).

### 0.3 Rate limiting general

Respuestas 429 con header `Retry-After`. Límites default (tunables por config del binario):

| Operación | Límite |
|-----------|--------|
| `POST /auth/magic-link` | 3/h por email + 10/h por IP |
| `POST /auth/login/email` | 10/h por IP |
| `POST /auth/login/password` | 5 fallos/15min por email+IP (backoff exponencial) |
| `POST /auth/qr/start` | 60/h por IP |
| `POST /classrooms/join`, `POST /guest/sessions` | 20/h por IP |
| Creación de sandboxes (runner) | presupuesto de contenedores por escuela/sala (§8) |

---

## 1. Setup wizard (CU-12)

Solo accesible mientras la instancia NO está configurada. Una vez configurada, cualquier llamada devuelve `409 setup_already_done` y `GET /setup/status` reporta `{configured: true}` para que `/setup` redirija al login.

### GET /setup/status
**Auth:** pública.
```json
{ "configured": false }
```
`200` siempre.

### POST /setup/school
**Auth:** pública (solo pre-setup).
```json
{ "name": "Escuela EPET 24" }
```
- `201` → `{ "school_id": "01J...", "name": "..." }`
- Retomable si el wizard se interrumpe (el estado parcial persiste).

### POST /setup/admin
**Auth:** pública (solo pre-setup, requiere school creada).
```json
{ "name": "Ana García", "email": "ana@escuela.edu.ar", "password": "…" }
```
- `201` + Set-Cookie sesión `staff` → `{ "user": {...}, "school": { "global_code": "ESCUELA-7K2M" } }` (código global generado automáticamente, cero decisiones extra en el wizard).
- `400 validation_error` (password débil), `409 setup_already_done`.

---

## 2. Auth

> Sección normativa. Las condiciones numeradas [C1]–[C6] son las 6 condiciones obligatorias de §13.9.5.

### 2.1 Login unificado dos pasos (docente/director — CU-7)

#### POST /auth/login/email — paso 1: detección de rol
**Auth:** pública. **Rate limit:** 10/h por IP.

```json
{ "email": "ana@escuela.edu.ar" }
```

`200` (siempre, salvo rate limit):
```json
{ "next": "password" }   // rol docente/director → pantalla password
{ "next": "magic_link" } // rol alumno → flujo §2.2
```

> **Nota de seguridad:** la detección de rol es parte del UX aprobado (§13.9.2) y revela la clase de cuenta existente. Mitigación: rate limit agresivo + errores genéricos en paso 2 + nunca revelar existencia en el envío de magic link (§2.2).

#### POST /auth/login/password — paso 2
**Auth:** pública. **Rate limit:** 5 fallos/15min por email+IP.

```json
{ "email": "ana@escuela.edu.ar", "password": "…" }
```

- `200` + Set-Cookie sesión `staff` → `{ "user": { "id": "...", "name": "...", "email": "...", "role": "docente" } }`
- `401 invalid_credentials` — mensaje genérico ("email o contraseña incorrectos"), idéntico para email inexistente y password mala.
- `403 account_disabled` — docente desactivado: *"tu cuenta está desactivada; contactate con la dirección de la escuela"* (deriva al director, no loop de login, CU-15).

Comparación de password: bcrypt/argon2id. Timing constante aunque el email no exista.

### 2.2 Magic link del alumno (CU-2, CU-17)

#### POST /auth/magic-link — solicitar link
**Auth:** pública. **Rate limit:** 3/h por email + 10/h por IP.

```json
{ "email": "juan@escuela.edu.ar", "context": "login_pc" | "pairing_pwa" }
```

`202` **siempre con este body, exista o no la cuenta** ([C2]/anti-enumeración):

```json
{ "sent": true, "message": "Si el email corresponde a una cuenta, recibís un enlace en unos minutos." }
```

Notas:
- Si la cuenta existe, se envía un email con link `https://<dominio-oficial>/auth/consume?token=<≥128 bits>`. **TTL ≤10 minutos, single-use.**
- `context:"pairing_pwa"` es la ÚNICA vía válida para emparejar el celular inicial ([C2]: pairing SOLO vía magic link validado por email). El token consume desde un dispositivo nuevo lo registra como dispositivo emparejado.
- `context:"login_pc"` produce sesión `pc_temporal` en la PC que consume (fallback oficial del QR inverso, CU-2). **[C2-observación]:** si el consume llega desde una PC que ya tiene sesión `pc_temporal` activa, la sesión previa se invalida antes de emitir la nueva (evita que un magic link abierto desde el webmail de una PC compartida deje sesión persistente en máquina ajena).

#### GET /auth/consume?token=... — validar y abrir sesión
**Auth:** pública. El frontend invoca; el backend hace redirect final a la app.

- `200` + Set-Cookie (clase según `context`) → `{ "user": {...}, "session_kind": "pwa"|"pc_temporal" }`
- `410 token_expired` (>10 min), `410 token_used` (ya consumido — single-use atómico), `400 invalid_token`.
- `409 device_limit_reached` si `context:"pairing_pwa"` y la cuenta ya tiene 3 dispositivos emparejados ([C5]). El token igualmente queda consumido.

### 2.3 QR inverso (CU-16) — login en PC compartida

Flujo: PC pide pairing session → muestra QR → celular autenticado escanea → confirma viendo identidad de la PC → claim atómico → PC obtiene sesión temporal.

Condiciones [C3]: single-use, TTL ≤90 s, entropía ≥128 bits, URL del dominio oficial (nunca shortener), regeneración ~60 s, confirmación en celular mostrando identidad de la PC. **[C3-observación]:** el `device_label` es autodeclarado por quien llama a `/qr/start` (un atacante puede generar un QR legítimo con label creíble y pegarlo sobre el monitor real). Mitigación: la pantalla de confirmación del celular muestra **IP de origen + timestamp** junto al label, tratando el label como decorativo, no como prueba de identidad.

#### POST /auth/qr/start — la PC crea la sesión de emparejamiento
**Auth:** pública (es justamente el login). **Rate limit:** 60/h por IP.

```json
{ "device_label": "LAB-PC07" }   // opcional; hostname legible que verá el alumno
```

`201`:
```json
{
  "pairing_id": "01J...",
  "qr_url": "https://ocicat.escuela.edu.ar/qr/tOKeN_128bits_base64url",
  "expires_in": 90,
  "refresh_after": 60
}
```

Notas de seguridad:
- `pairing_id` y token del QR son aleatorios ≥128 bits e independientes. El QR lleva el token; `pairing_id` solo sirve para poll del estado.
- `qr_url` SIEMPRE dominio oficial de la instancia ([C3], anti-quishing). Single-use: confirmar lo consume; vencido → `expired`.
- Regeneración: la PC vuelve a llamar este endpoint (~60 s o al vencer).

#### GET /auth/qr/{pairing_id}/status — la PC consulta estado (poll)
**Auth:** pública (polling barato; long-poll opcional via `?wait=10`).

`200`:
```json
{ "status": "waiting" }    // | "scanned" | "confirmed" | "expired" | "denied"
```
- En `confirmed` la respuesta incluye Set-Cookie con la **sesión `pc_temporal`** del alumno ([C4]: TTL ≤60 min, inactividad ≤10 min) y `{ "status": "confirmed", "user": { "name": "...", "role": "alumno" } }`.
- `scanned` permite a la PC mostrar "revisá tu celular…".
- `GET` no muta estado; el claim ocurre solo en `/confirm`.

#### POST /auth/qr/scan — el celular autenticado registra el escaneo
**Auth:** alumno con sesión activa (PWA emparejada).

```json
{ "qr_token": "tOKeN_128bits_base64url" }
```

`200` → datos para la pantalla de confirmación (identidad de la PC, [C3]):
```json
{ "pairing_id": "01J...", "device_label": "LAB-PC07", "requested_at": "2026-08-22T14:03:11Z" }
```
- `404 not_found`, `410 token_expired` ("este código ya no sirve, mirá el nuevo").
- Marca la sesión como `scanned` y asocia la cuenta del celular. La PC aún no sabe quién es.

#### POST /auth/qr/{pairing_id}/confirm — confirmación del celular (claim atómico)
**Auth:** alumno (mismo dispositivo/cuenta que hizo `/scan`).

Body: vacío.

- `200 {}` — claim exitoso. Transición `scanned→confirmed` exactamente una vez; claims concurrentes → uno gana, el resto `409 already_claimed` (sin doble claim, CU-16).
- Rechazo explícito: `POST /auth/qr/{pairing_id}/deny` → `200 {}`, la PC ve `denied` y **nunca inicia sesión**.
- `410 token_expired` si pasaron los 90 s entre scan y confirm.

### 2.4 Sesiones y dispositivos (CU-6, CU-17)

#### GET /sessions
**Auth:** cualquiera autenticado. Lista sesiones activas de la propia cuenta.

```json
{ "sessions": [
  { "id": "01J...", "kind": "pwa", "device_label": "Pixel de Juan", "created_at": "...", "last_seen_at": "..." },
  { "id": "01J...", "kind": "pc_temporal", "device_label": "LAB-PC07", "created_at": "...", "last_seen_at": "..." }
] }
```

#### DELETE /sessions/current — logout
`204`. Revoca la sesión actual y limpia la cookie.

#### DELETE /sessions/{id}
`204`. Revoca una sesión puntual (desvincular celular perdido = borrar su sesión `pwa`; libera cupo del límite de dispositivos).

#### POST /sessions/close-others — "Cerrar otras sesiones" (perfil.html)
`204`. Revoca todas menos la actual.

---

## 3. Aulas (CU-3, CU-6, CU-8, CU-10)

Recurso `classroom`. Microcopy UI: **"Aulas"**, nunca "Salas".

### POST /classrooms
**Auth:** docente, director.
```json
{ "name": "Programación 5A", "course": "5°", "shift": "mañana" }
```
`201` → classroom completo, incluye `"join_code": "PROG5A"` generado automáticamente.

### GET /classrooms
**Auth:** cualquiera autenticado. Scope por rol:
- alumno: sus aulas + `pending_assignments` count ("3") o `"up_to_date": true` (CU-6);
- docente: las que dicta + stats (`students_count`, `active_assignments`, `running_sandboxes`) para el dashboard (CU-7);
- director: TODAS las aulas de la escuela (vista global, §13.9.2) + nombre del docente de cada una.

`200` → `{ "classrooms": [...] }`

### GET /classrooms/{id} · PATCH · DELETE
**Auth:** docente dueño, director. `DELETE` archiva (entregas quedan archivadas, patrón CU-6). `200` / `200` / `204`.

### GET /classrooms/{id}/join_code
**Auth:** docente dueño, director. `200` → `{ "code": "PROG5A" }`

### POST /classrooms/{id}/join_code/rotate — rotar código (CU-10)
**Auth:** docente dueño, director. Body: vacío.

`200` → `{ "code": "X7PROG2" }` — **el código anterior deja de funcionar AL INSTANTE** (aviso doble en UI antes de confirmar; fuga de código fuera del aula).

### POST /classrooms/join — entrar con código (CU-3)
**Auth:** alumno.

```json
{ "code": "PROG5A" }
```

- `200` → `{ "classroom": {...} }` (queda inscripto; aparece en Mis Aulas).
- `404 invalid_code` — código inválido, rotado o desactivado: *"este código no funciona; pedile el nuevo a tu docente"*.
- `409 already_member`.
- Nota: el código es *invitación*, no identidad (§11.2). Rate limit 20/h por IP (fuerza bruta de códigos de 6–8 caracteres).

### DELETE /classrooms/{id}/membership/me — abandonar aula (CU-6)
**Auth:** alumno miembro. `204`. Entregas quedan ARCHIVADAS (el docente todavía puede verlas); modal previo en UI.

### GET /classrooms/{id}/students
**Auth:** docente dueño, director. `200` → lista con `name`, `email`, `status: invited|active`, entregas totales.

### DELETE /classrooms/{id}/students/{user_id} — baja de alumno (CU-18 alt.)
**Auth:** docente dueño, director. `204`. Entregas archivadas, cuenta NO borrada (borrado real es endpoint aparte, ver §6.5).

---

## 4. Consignas (CU-9)

Viven DENTRO del aula (no hay creación global en nav, §13.9.2). Recurso `assignment`.

### POST /classrooms/{id}/assignments
**Auth:** docente dueño, director.
```json
{
  "title": "TP2 — Listas",
  "instructions": "texto markdown",
  "attachment_ids": ["01J..."],
  "runtime": "python",          // python | web | cpp | arduino (catálogo §10.2)
  "due_at": "2026-09-01T23:59:00Z",   // opcional
  "attempts": { "mode": "unlimited" },  // unlimited | { "mode": "limited", "max": 3 } | { "mode": "one" }
  "late_policy": "allowed"      // allowed (marcadas tardías) | closed (cierra al vencer)
}
```
`201`. Publicación es inmediata: los alumnos la ven al instante.

Labels docentes en UI (no en API): "entorno del ejercicio", "qué pueden usar los alumnos" (§13.3).

### GET /classrooms/{id}/assignments
**Auth:** miembros (alumno ve activas con vencimientos), docente dueño/director ven todas + métricas.

### GET/PATCH/DELETE /assignments/{id}
**Auth:** docente dueño, director (lectura también para miembros del aula).
- `PATCH`: config de entrega modificable **en cualquier momento, incluso con entregas existentes** (§9.1): bajar intentos no invalida intentos ya hechos; cerrar tardías aplica desde ese momento.
- `DELETE`: `204` (soft-delete; entregas conservadas).

### GET /assignments/{id}/stats — métricas del listado (CU-9 paso 4)
**Auth:** docente dueño, director.
```json
{ "total_students": 28, "delivered": 24, "tested_ok": 20, "test_errors": 2, "late": 2, "missing": 4 }
```

### Adjuntos de consigna (CU-9)
`POST /assignments/attachments` — **Auth:** docente dueño, director. `multipart/form-data` con `file` (máx **10 MB/archivo**). `201` → `{ "attachment_id": "01J...", "filename": "...", "size_bytes": ... }`. El id se usa en `attachment_ids` del create de assignment. Los adjuntos sin assignment asociado se purgan a las 24 h (GC).

---

## 5. Entregas (CU-5 — núcleo del MVP)

Wizard 3 pasos (§13.2): consigna → archivos → probar (opcional) → checkpoint explícito → entregado. Recurso `submission` = UN intento.

Estados de entrega: `draft` → `delivered` (+ flags derivados: `tested_ok` | `test_error` | `late`).

### 5.1 Subir archivos (paso 2)
#### POST /assignments/{id}/submissions/files
**Auth:** alumno miembro del aula. `multipart/form-data` (uno o más archivos; .zip, .py, index.html, etc.).

`201`:
```json
{ "submission_id": "01J...", "files": [ { "id": "01J...", "name": "tp2.py", "size": 3412 } ], "state": "draft" }
```
- Crea (o reusa) la submission en estado `draft` del intento en curso.
- `400 file_too_large` (>10 MB/archivo), `400 validation_error` (tipo prohibido).

### 5.2 Probar en sandbox (paso 3, opcional)
Lanza sandbox job efímero ligado a la draft: `POST /sandboxes` con `"purpose": "submission_test", "submission_id": "..."` → ver §7. El resultado de la prueba queda asociado al snapshot final.

### 5.3 Checkpoint explícito y entrega final (paso 4)
#### POST /submissions/{submission_id}/deliver
**Auth:** alumno propietario.
```json
{ "confirm_attempt": 2 }
```

`confirm_attempt` = número de intento que el alumno vio en pantalla ("¿Entregar? (intento 2 de 3)"). Concurrency guard: si mientras tanto el docente bajó el límite o hubo otro intento, el server detecta el mismatch.

- `201` → submission `delivered`:
```json
{ "id": "01J...", "attempt_number": 2, "state": "delivered",
  "late": false,
  "delivered_at": "2026-08-22T15:04:00Z",
  "attempts_remaining": 1,
  "last_test_result": { "run_id": "01J...", "exit_code": 0 } }
```
- `409 attempts_exhausted` — sin intentos restantes según config vigente.
- `409 deadline_passed` — vencida y `late_policy:"closed"` (§9.1).
- `409 attempt_conflict` — `confirm_attempt` no coincide con el intento real (UI desactualizada; el cliente refresca).
- Snapshot inmutable: archivos + resultado de prueba se copian al entregar. Reintento = nueva submission draft.

### 5.4 Consultas
- `GET /assignments/{id}/submissions/me` — **Auth:** alumno. Historial de SUS intentos (#N, exitosa/falló/tardía).
- `GET /submissions/{id}` — **Auth:** propietario, docente dueño, director. Snapshot completo.
- `GET /assignments/{id}/submissions?filter=late|missing|error` — **Auth:** docente dueño, director. Vista de corrección.

---

## 6. Materiales (CU-1, CU-4, CU-10)

Biblioteca estilo Drive con preview inline (§9.3). Recurso `material`.

Visibilidad (`visibility`):
- `"public"` — visible para anónimos, invitados y miembros. Es lo que alimenta la vista pública.
- `"school"` — invitados CON código global + miembros (lo que el código global desbloquea a nivel escuela, §13.7).
- `"classroom"` — solo miembros del aula (+director).

Las consignas y entregas NUNCA tienen visibilidad mayor a `classroom` — regla dura, no configurable.

### GET /materials?scope=public|mine|classroom:{id}&subject=...
**Auth:** pública (filtra según quien pregunta: anónimo → `public`; invitado → `public+school`; alumno/docente → lo suyo; director → todo).

`200` → `{ "materials": [ { "id", "title", "type": "pdf|video|image|office|other", "visibility", "subject", "preview_available": true, "uploaded_by", "created_at" } ] }`

### POST /classrooms/{id}/materials
**Auth:** docente dueño, director. Creación vive DENTRO del aula (§13.9.2).
`multipart/form-data`: `file`, `title`, `visibility`, `subject?`.
`201` → material completo.

### GET /materials/{id} — metadata + página propia
**Auth:** según visibility. `403 forbidden` / `404 not_found` indistinguibles para quien no tiene acceso.

### GET /materials/{id}/file — stream con soporte Range (preview inline y descarga)
**Auth:** ídem. `?download=1` fuerza `Content-Disposition: attachment`. Office: sin preview embebido (card con metadata + "Abrir").

### PATCH/DELETE /materials/{id}
**Auth:** autor (docente), director. `200` / `204`.

---

## 7. Sandboxes y ejecuciones (CU-11, CU-5 paso 3)

Templates de entorno (§10.2): catálogo por instancia (`python/numpy`, `bun/react`, `cpp/sqlite`, `arduino`…), cada uno define imagen allowlist, comando start default editable, modo (job/service), puertos. Defaults sensatos hardcoded; **config por sala MÍNIMO MVP (G4, CU-10.4):** `PATCH /classrooms/{id}/settings` (**Auth:** docente dueño, director) con `{ "allowed_templates": ["python/numpy", ...], "custom_dockerfile_enabled": false }` — default: catálogo completo de la instancia + toggle OFF. Restricciones más granulares quedan para v2 (§13.4); open question Q4 queda resuelta a este mínimo.

Estados del run (une §2.3 máquina de estados + §13.2 estados visibles):

```text
queued ──> downloading_image ──> starting ──> ready ──> running ──> succeeded
   │                                                          │
   ├──> rejected (presupuesto agotado, hard limit)            ├──> failed (error | timeout)
   └── (espera de turno visible: queue_position)              └──> cleaned (contenedor liberado;
                                                                  registro persiste si retention=historical)
```

Modo `service`: queda `running` con `service_url` hasta stop o expiración.

### GET /templates?classroom_id=
**Auth:** alumno miembro, docente, director. Devuelve los templates permitidos para esa sala ("qué pueden usar los alumnos"). `200` → `{ "templates": [ { "id", "label": "Python + NumPy", "mode_default": "job", "runtimes": [...] } ] }`

### POST /sandboxes — crear job efímero (o service)
**Auth:** alumno miembro, docente, director. **Rate limit:** presupuesto de contenedores (§0.3).

```json
{
  "template_id": "tpl_python_numpy",
  "packages_extra": "pandas matplotlib",   // texto plano, modo default (§13.2)
  "mode": "job",                            // job (corre y se apaga) | service
  "start_command": "python main.py",        // opcional, edita el default del template
  "purpose": "standalone"                   // | "submission_test" (ligado a wizard CU-5)
  , "submission_id": "01J..."               // requerido si purpose=submission_test
  , "retention": "historical"               // historical (default, registro re-accedible) | ephemeral
}
```

`202 Accepted` → run creado en `queued`:
```json
{ "sandbox_id": "01J...", "run_id": "01R...", "status": "queued", "queue_position": 2,
  "budget_note": "2 de 4 sandboxes prendidos" }
```

- `202 Accepted` + `queue_position` cuando el presupuesto está lleno pero hay lugar en cola: el run **entra en cola** (§13.2). NO existe `429 budget_full` (un 429 implicaría "no se creó nada", incompatible con "espera su turno").
- `503 capacity_unavailable` solo si la cola dura también está llena: no se creó nada, reintentar más tarde.
- `400 validation_error` si packages_extra viola allowlist del template.
- Plan B sin Docker (riesgo #1 mitigado, §13.4): si el runner está caído, este endpoint responde `503` pero la ENTREGA de archivos (§4) funciona igual.

### GET /sandboxes — listado
**Auth:** variable (como materiales):
- alumno/docente: propios/de sus aulas con estado actual;
- invitado con código y anónimo: registros `historical` con visibility `public`/`school` — ven estado corriendo/detenido y autor (CU-1 paso 3, CU-14 paso 3), lectura only.

### GET /sandboxes/{id} — registro histórico (metadata + logs + artifact)
**Auth:** lectura según visibility; `historical` es re-accedible y re-instanciable (§2.3). `ephemeral` → `404` tras limpieza.

### POST /sandboxes/{id}/instantiate — relanzar desde el registro
**Auth:** autor, docente del aula, director (federado post-MVP). Igual que `POST /sandboxes` pero desde repo/artifact cacheado. `202` → run nuevo.

### GET /runs/{run_id} — estado actual
```json
{ "run_id": "01R...", "sandbox_id": "01J...", "status": "downloading_image",
  "exit_code": null, "service_url": null, "started_at": "...", "history":
  [ { "n": 1, "outcome": "success" }, { "n": 2, "outcome": "timeout" } ] }
```
Historial de ejecuciones #N (exitosa/falló/timeout) para `sandbox-detalle.html` (CU-11 paso 4). Modo service web → iframe apunta a `service_url` (`https://<host>/s/<id>`).

### GET /runs/{run_id}/logs — streaming de logs
**Auth:** autor, docente, director (invitado: solo logs de runs históricos terminados, lectura).

Respuesta `text/event-stream` (**SSE canónico**; WS alternativo queda como open question Q3):
```text
event: log
data: {"stream":"stdout","line":"hola mundo"}

event: status
data: {"status":"running"}

event: exit
data: {"exit_code":0,"status":"succeeded"}
```

### POST /runs/{run_id}/stop
**Auth:** autor, docente, director. `200` → `{ "status": "cleaned" }`. Detiene y limpia el contenedor; el registro persiste según retention.

---

## 8. Notificaciones (badge MVP, §13.9.2)

Campana con badge en nav registrada. Fuentes MVP: nueva consigna publicada en mi aula, consigna próxima a vencer, entrega recibida (para el docente), resultado de prueba disponible.

### GET /notifications/unread-count
**Auth:** cualquiera autenticado. `200` → `{ "count": 3 }` (poll liviano para el badge; <300 KB/página obliga a payload mínimo).

### GET /notifications?unread=true&limit=20
**Auth:** ídem. `200` → `{ "notifications": [ { "id", "type": "assignment_published", "title", "body", "link": "/classrooms/.../assignments/...", "read": false, "created_at" } ] }`

### POST /notifications/{id}/read · POST /notifications/read-all
`204`.

---

## 9. Gestión escolar — director (CU-12, CU-13, CU-15, CU-18)

Panel `admin.html`. Todo lo de esta sección requiere rol **director** (único gestor del código global — un docente jamás puede regenerarlo, CU-13).

### 9.1 Código global de escuela

#### GET /school
`200`:
```json
{ "name": "Escuela EPET 24",
  "global_code": { "code": "ESCUELA-7K2M", "active": true },
  "stats": { "teachers": 2, "classrooms": 5, "students": 87, "public_materials": 120 } }
```

#### POST /school/global-code/regenerate
Body: vacío. `200` → `{ "code": "ESCUELA-Q9XP" }`. **El código anterior muere al instante**; los invitados viejos quedan afuera (patrón rotar código de sala, doble confirmación en UI).

#### DELETE /school/global-code — desactivar
`204`. Estado: sin código vigente (se puede generar uno nuevo cuando quiera; nadie entra como invitado hasta entonces).

### 9.2 Docentes (CU-15)

#### GET /school/teachers
`200` → `[ { "id", "name", "email", "status": "active|disabled", "classrooms_count", "created_at" } ]`

#### POST /school/teachers — crear cuenta docente
```json
{ "name": "Carlos Pérez", "email": "carlos@escuela.edu.ar",
  "credential": { "type": "password", "password": "temp1234!" }
                | { "type": "magic_link" } }
```
`201` → docente activo. El director le pasa credenciales/enlace. `409 email_already_exists` — **emails duplicados rechazados en alta** ([C5]).

#### POST /school/teachers/{id}/reset-credential — recovery
Igual shape que `credential` arriba. `200`. Mismo patrón que recovery de alumnos (CU-10/CU-15).

#### POST /school/teachers/{id}/disable · POST /school/teachers/{id}/enable
`200`. Desactivar: no podrá entrar (`403 account_disabled` en login); **sus aulas y consignas quedan intactas pero congeladas** (lectura para alumnos, sin consignas nuevas — decisión mínima MVP; reasignación a v2).

### 9.3 Alta de alumnos — registro CERRADO (CU-18, [C1])

NO existe registro self-service ([C1]: decisión de arquitectura más importante de la auditoría). Puerta: docente del aula o director. Data minimization: solo `name`/`course`/`email` ([C6]).

#### POST /classrooms/{id}/students/import — import CSV (dos pasos)
**Auth:** docente dueño del aula, director. (Ídem `students/invite` y `resend-magic-link`.)
Paso 1 — preview (parse sin commit):
`multipart/form-data`: `csv` (nombre + email por fila).

```json
{ "dry_run": true }
```

`200` → preview fila por fila (CU-18: duplicados/inválidos rechazados fila por fila con motivo):
```json
{ "import_token": "01J...",
  "rows": [
    { "line": 1, "name": "Juan López", "email": "juan@escuela.edu.ar", "status": "ok" },
    { "line": 2, "name": "María Díaz", "email": "juan@escuela.edu.ar", "status": "duplicate", "reason": "email repetido en el CSV (línea 1)" },
    { "line": 3, "name": "Pedro Sosa", "email": "pedro@", "status": "invalid_email" },
    { "line": 4, "name": "Ana Ruiz", "email": "ana@escuela.edu.ar", "status": "exists", "reason": "ya tiene cuenta en la escuela" }
  ],
  "summary": { "ok": 21, "rejected": 4 },
  "warnings": ["detectadas casillas compartidas (mismo email en varias filas): preferí casillas individuales"]
}
```
Paso 2 — commit: mismo multipart + `{"dry_run": false, "import_token": "..."}`. **Regla de commit:** solo se dan de alta las filas `ok`; las `duplicate`/`invalid_email`/`exists` se omiten (no bloquean) y el docente las ve en el resumen. Límite: **≤200 filas por import**. `201` → `{ "created": 21, "omitted": 4 }` + detalle por fila; cada alumno queda con cuenta activa **sin password** + magic link de primer acceso generado (enviado o pasado por el docente).

#### POST /classrooms/{id}/students/invite — invitación individual
**Auth:** docente dueño del aula, director.
```json
{ "name": "Lucía Fernández", "email": "lucia@escuela.edu.ar", "send_invite": true }
```
`201` → `{ "student": {...}, "first_magic_link_sent": true }`. Alta al momento (mitad de año). `409 email_already_exists`.

El docente puede reenviar el magic link manualmente (recovery): `POST /students/{user_id}/resend-magic-link` (**Auth:** ídem) → `202` (respuesta genérica anti-enumeración).

#### DELETE /users/{user_id} — borrado real de cuenta ([C6])
**Auth:** director únicamente. `204`. Borrado efectivo de datos personales (entregas quedan anonimizadas como registro escolar). Página de privacidad simple en castellano fuera del scope API.

### 9.4 Vista global de aulas
Cubierto por `GET /classrooms` (rol director devuelve todas + stats) y `GET /school/stats`. No hay endpoint dedicado extra.

### 9.5 Impersonación administrativa (endpoint SEPARADO, auditado)

Resuelve §13.9.2.5 ("ve todo y gestiona sin credenciales de docente") vía impersonación documentada — alternativa de acceso-directo-por-rol queda en open questions (Q1).

#### POST /admin/impersonations
**Auth:** director.
```json
{ "user_id": "01J...", "reason": "configurar plantilla del aula 5A a pedido del docente" }
```
`201` + Set-Cookie nueva sesión marcada `impersonated_by=<director_id>` → actúa COMO ese usuario en todos los endpoints. `reason` obligatorio (auditoría).

- Banner UI obligatorio "estás actuando como X (admin)". Logout de impersonación ≠ logout del director.

#### DELETE /admin/impersonations/current
`204` — vuelve a la sesión del director.

#### GET /admin/audit-log?actor=&action=&from=&to=
**Auth:** director. Registro inmutable de acciones sensibles: impersonaciones (inicio/fin + reason), rotaciones de códigos (sala y global), altas/bajas de docentes y alumnos, borrados de cuenta, cambios de credenciales.
```json
{ "entries": [ { "id", "actor_id", "action": "impersonation.start", "target_user_id", "reason", "ip", "created_at" } ] }
```

---

## 10. Invitados — código global de escuela (CU-14)

### POST /guest/sessions — entrar con código global
**Auth:** pública. **Rate limit:** 20/h por IP.

```json
{ "code": "ESCUELA-7K2M" }
```

- `201` + Set-Cookie sesión de invitado (rol `guest`, read-only) → `{ "school_name": "Escuela EPET 24" }` (aterriza en vista "Escuela <nombre>", CU-14).
- `404 invalid_code` — inválido o regenerado: *"este código ya no funciona; pedile el nuevo a la escuela"*.
- Un alumno ya registrado no necesita esto (ya ve más); el copy UI lo aclara.

Capacidades del invitado (y del anónimo sobre contenido `public`): **VER y DESCARGAR únicamente**:
- `GET /materials` + `/materials/{id}` + `/materials/{id}/file` (scope public/school),
- `GET /sandboxes` + `/sandboxes/{id}` + logs históricos (lectura),
- `GET /school/public` → `{ "name": "..." }` (para la vista de escuela).

Cualquier mutación o acceso a consignas/entregas/materiales `classroom` → `403 guest_read_only` con copy: *"las consignas son privadas del curso; para participar necesitás que tu docente te dé de alta"* (límite claro CU-1/CU-14). Link `ocicat.edu/s/CODIGO` compartido ejecuta este mismo endpoint.

---

## 11. Perfil

### GET /me
**Auth:** cualquiera autenticado. `200` → `{ "id", "name", "email", "role", "school_name", "devices_paired": 2 }` (la nav por rol §13.9.x se decide con `role`).

### PATCH /me
**Auth:** ídem. Solo `name` (email es identificador inmutable; cambiarlo = proceso manual del admin). Data minimization [C6].

---

## 12. Cobertura CU ↔ endpoints

| CU | Flujo | Endpoints principales |
|----|-------|----------------------|
| 1 | Invitado explora y choca con límite | `GET /materials`, `GET /sandboxes`, `POST /guest/sessions` (opcional), errores `403 guest_read_only` |
| 2 | Alumno sin celular: magic link en PC | `POST /auth/login/email` → `next:magic_link`, `POST /auth/magic-link` (context login_pc), `GET /auth/consume` |
| 3 | Entrar a aula con código | `POST /classrooms/join` |
| 4 | Ver materiales del aula | `GET /materials?scope=classroom:*`, `GET /materials/{id}`, `/file` |
| 5 | Wizard de entrega | `POST /assignments/{id}/submissions/files`, `POST /sandboxes` (purpose submission_test), `POST /submissions/{id}/deliver` (checkpoint) |
| 6 | Mis aulas / abandonar / sesiones | `GET /classrooms`, `DELETE /classrooms/{id}/membership/me`, `GET /sessions`, `POST /sessions/close-others` |
| 7 | Login docente + dashboard | `POST /auth/login/email` → `next:password`, `POST /auth/login/password`, `GET /classrooms` (stats) |
| 8 | Crear aula + compartir código | `POST /classrooms`, `GET /classrooms/{id}/join_code` |
| 9 | Crear consigna con config | `POST /classrooms/{id}/assignments` (attempts/late_policy), `GET /assignments/{id}/stats` |
| 10 | Materiales, alumnos, rotación | `POST /classrooms/{id}/materials`, `GET /classrooms/{id}/students`, `POST .../join_code/rotate`, recovery `POST /students/{id}/resend-magic-link` |
| 11 | Lanzar/seguir sandbox | `GET /templates`, `POST /sandboxes`, `GET /runs/{id}`, `GET /runs/{id}/logs` (SSE), `POST /runs/{id}/stop`, `POST /sandboxes/{id}/instantiate` |
| 12 | Wizard /setup director | `GET /setup/status`, `POST /setup/school`, `POST /setup/admin` |
| 13 | Gestión código global | `GET /school`, `POST /school/global-code/regenerate`, `DELETE /school/global-code` |
| 14 | Invitado con código global | `POST /guest/sessions`, lecturas públicas, `403 guest_read_only` |
| 15 | Gestión docentes | `GET/POST /school/teachers`, `POST .../{id}/reset-credential`, `POST .../{id}/disable|enable` |
| 16 | QR inverso en PC compartida | `POST /auth/qr/start`, `GET /auth/qr/{id}/status`, `POST /auth/qr/scan`, `POST /auth/qr/{id}/confirm\|deny` |
| 17 | Emparejamiento celular (magic link) | `POST /auth/magic-link` (context pairing_pwa), `GET /auth/consume`, `409 device_limit_reached`, `DELETE /sessions/{id}` |
| 18 | Alta de alumnos cerrada | `POST /classrooms/{id}/students/import` (dry_run→commit), `POST /classrooms/{id}/students/invite`, `DELETE /users/{id}` |

Cobertura: 18/18 CUs.

---

## 13. Preguntas abiertas

1. **Impersonación vs acceso directo por rol** (§13.9.2.5 deja ambas abiertas): este contrato especifica impersonación auditada. ¿Confirmamos esa opción, o el director simplemente pasa checks de docente sin cambio de identidad (más simple, pero el audit log pierde granularidad)?
2. **Enumeración en paso 1 del login**: la respuesta `next` distingue alumno de staff para emails existentes. ¿Aceptar la fuga parcial (rate-limit mitigando) o responder neutro siempre y detectar rol tras magic-link/password?
3. **Logs streaming: SSE vs WebSocket** — specifiqué SSE canónico (coincide con §4 viejo y es unidireccional). ¿Hace falta WS para algo del MVP?
4. **Dockerfile propio** (toggle §10.5): quedó post-MVP en API. Si entra, endpoints de build/lint aparte.
5. **Puerto/URL del modo service**: `/s/{sandbox_id}` heredado del diseño §2.3 — confirmar formato antes de implementar iframe.
6. **SMTP obligatorio**: magic link requiere email transaccional por instancia self-hosted (§11.3). ¿Fallback sin SMTP (docente genera link manual y lo pasa impreso)? Impacta CU-2/CU-17 en escuelas con SMTP bloqueado.
7. **Expiración del modo service**: TTL default de un service (¿horas? ¿fin de clase?) — define el runner, falta decidir.
8. **Legacy `backend/` Python**: prototipo FastAPI pre-rediseño; propongo eliminarlo para que nadie lo tome como referencia del contrato Go.
