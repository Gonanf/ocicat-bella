# Draft: Ocicat Bella MVP Vertical Slice

## Context
- DESIGN.md cerrado (824 líneas, Partes 0–7 completas, decisiones abiertas resueltas).
- Stack: Backend Go binario único (CGO_ENABLED=0, distroless <50MB), Turso/LibSQL store, Docker runner.
- Frontend: Astro + daisyUI sobre Cloudflare Pages. UI ligera para PC viejo.
- Federación: MVP usa relay Cloudflare como índice (no replica DB).
- Slice MVP objetivo: docente crea sala → alumno entra por código → ve sandboxes → corre sandbox job → ve logs.
- Pantallas core a mockup con open-design: /, /materials, /join, /sandboxes, /sandboxes/:id.

## Requirements (confirmed from user request)
- Planificar el slice MVP vertical (flujo end-to-end, no solo frontend).
- Pantallas core: Landing, Materiales+instaladores, Join, Sandboxes lista, Sandbox detalle+Run/logs.
- Para cada pantalla: propósito, elementos UI clave, datos que muestra (Sala, Sandbox, Alumno, ContainerRun del DESIGN.md), conexión con backend Go+Turso+Docker.
- Output: PLAN concreto (no código) listo para Sisyphus.
- Mencionar open-design MCP disponible para generar mockups HTML.

## Technical Decisions
- (pendiente) Confirmar: ¿Astro puro SSG o Astro híbrido con SSR para /sandboxes/:id (live logs SSE)?
- (pendiente) daisyUI v4 vs v5 + Tailwind v3 vs v4.
- (pendiente) ¿OAuth Google stub o flujo real? (DESIGN dice OAuth real, pero MVP slice)
- (pendiente) ¿Quién crea sandboxes en el MVP? (¿docente? ¿alumno? ¿seed?)
- (pendiente) Auth del docente en slice: ¿stub (botón "Login como docente demo") o real?

## Research Findings
- (pendiente) Astro + daisyUI best practice 2026
- (pendiente) open-design MCP availability
- (pendiente) Go SSE + Docker runner patterns

## Open Questions (for user)
1. ¿Quién crea sandboxes en el MVP slice? (seed admin, docente, o ambos)
2. ¿OAuth Google: stub (mock con sesión local) o real (necesita credentials)?
3. ¿Astro SSG puro o híbrido SSR (para /sandboxes/:id que necesita datos dinámicos)?
4. ¿El alumno puede crear sandbox en el slice o solo ejecutar los existentes?
5. ¿Upload de material nuevo (POST /materials) entra al slice o lo dejamos solo lectura?

## Scope Boundaries
- INCLUDE:
  - Backend: endpoints mínimos para el flujo end-to-end
  - Frontend: 5 pantallas core + cliente API tipado
  - Setup wizard mínimo (puede ser solo landing placeholder)
  - SSE streaming de logs
- EXCLUDE (post-MVP):
  - Federación real (solo stub de peers)
  - Modo service (solo job)
  - Firecracker/unikernel
  - Materiales upload (solo lectura si entra)
  - Instaladores (Modo A)
  - Team sandbox (Fase 2)

## Notes
- "Slice vertical" = end-to-end thin, no broad horizontal features.
- El MVP completo está descrito en DESIGN.md Parte 7. Este plan = primer sub-slice ejecutable.
- Plan debe ser AUTO-EJECUTABLE por Sisyphus: cada task con QA scenarios verificables.
