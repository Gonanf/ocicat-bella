# Draft: Ocicat Bella MVP Vertical Slice Plan

## Requirements (confirmed)
- [Flow]: docente crea sala (OAuth Google stub) -> alumno entra por codigo de sala -> ve lista de sandboxes -> corre sandbox (modo job) -> ve logs
- [Screens]: Landing (/), Materiales+instaladores (/materials), Join por codigo (/join), Sandboxes lista (/sandboxes), Sandbox detalle + Run/logs (/sandboxes/:id)
- [Tech]: Astro + daisyUI frontend, connecting to backend Go+Turso+Docker (per DESIGN.md)
- [Models]: Based on DESIGN.md sections 3.1 (Go structs) and 3.2 (TS types)
- [API]: Based on DESIGN.md section 4 (API REST) + inferred auth/sala endpoints for MVP flow

## Technical Decisions
- [Frontend Framework]: Astro (chosen per DESIGN.md 3.2, ideal for old PCs, minimal client JS)
- [UI Library]: daisyUI (chosen per DESIGN.md 3.2 as lightweight alternative to Shadcn)
- [State Management]: Simple fetch/store per screen with localStorage for auth tokens
- [Auth]: 
  - Docente: OAuth Google flow (standard endpoints)
  - Alumno: POST /salas/join with codigo to get token de sesión efímero
  - All API requests use Authorization: Bearer <token> header
- [Real-time logs]: Server-Sent Events (SSE) from /runs/:id/logs endpoint
- [Sala Context]: Token contains sala_id for alumno; docente can select sala context
- [Test Strategy]: 
  - Infrastructure exists: NO (new Astro project)
  - Automated tests: YES (Tests after implementation approach)
  - Framework: Vitest (chosen for Astro compatibility, fast, minimal config)
  - Approach: Write tests after implementing each screen/component

## Research Findings
- [DESIGN.md 0.6]: Docente OAuth Google, alumno entra por código de sala (token de sesión efímero)
- [DESIGN.md 0.6]: Alumno identidad local a sala (anon, rastreable dentro de sala) -> Alumno struct with sala_id
- [DESIGN.md 0.6]: Visibilidad: TODOS los alumnos pueden VER todo (su sala, otras salas de su escuela, sandboxes federados de otras escuelas), pero solo pueden SUBIR en la sala que les permiten
- [DESIGN.md 0.6]: Código de sala por defecto abierto, opción de cupar seats
- [DESIGN.md 2.4]: Rate Limiter en EventBus para frenar abusos antes de tocar Docker
- [DESIGN.md 2.5]: Algoritmo Launch(sandbox) paso a paso
- [DESIGN.md 3.1]: Modelos Go: Alumno (with sala_id), Material, Sandbox (with SalaID), ContainerRun, FederationPeer
- [DESIGN.md 3.2]: Tipos TypeScript equivalentes
- [DESIGN.md 4]: API REST endpoints detallados including:
  - GET/POST /materials
  - GET/POST /sandboxes/:id (with sala_id in POST body)
  - POST /sandboxes/:id/run
  - GET /runs/:id
  - GET /runs/:id/logs (SSE)
  - POST /runs/:id/stop
- [DESIGN.md 5.2]: Defensa en profundidad, default deny, aislamiento por escuela/sala
- [Current State]: Backend is Python/FastAPI with minimal models, but plan should target Go backend per DESIGN.md

## Open Questions
- [Auth]: Exact OAuth Google endpoints (standard flow assumed)
- [Sala endpoints]: Need to define POST /salas for docente to create sala, GET /salas/:code for validation
- [Join Flow]: Alumno enters codigo -> POST /salas/join -> gets token -> uses token for subsequent requests
- [Materials]: "Instaladores" are downloadable executables/installers listed as materials with tipo="instalador"
- [Backend]: Plan assumes API contract per DESIGN.md; implementation can adapt existing Python backend

## Scope Boundaries
- INCLUDE: 
  - Frontend screens for the specified flow
  - API integration per DESIGN.md endpoints + inferred auth/sala endpoints
  - Real-time logs via SSE from /runs/:id/logs
  - Basic UI with daisyUI components (buttons, cards, inputs, modals, tooltips)
  - Responsive design for old PCs (mobile-first, minimal JS)
  - Auth state management (login/logout, token storage)
  - Error handling and loading states
- EXCLUDE:
  - Full federacion discovery (though sandbox listing shows federated ones visible per visibility rules)
  - Advanced auth (2FA, password reset, etc.)
  - Plugin system
  - Firecracker/unikenel backend (Docker only for MVP)
  - Setup wizard (though /setup route mentioned, not in core flow)
  - Material management beyond listing (no create/edit in MVP)
  - School management (alumno/escuela data comes from token context)