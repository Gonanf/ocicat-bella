# ocicat-bella

> Proyecto de Gonanf — colección personal.
> **Lenguaje principal (GitHub):** HTML · **URL:** https://github.com/Gonanf/ocicat-bella

## Qué es

Este repositorio forma parte de la colección de **Gonanf / Gabriel Solotorevsky** clonada en `/run/media/chaos/terciario/proyectos/ocicat-bella`.

> **Nota:** README original preservado abajo en la sección "README original".

- **Path absoluto:** `/run/media/chaos/terciario/proyectos/ocicat-bella`
- **Estado git:** último commit `2026-08-23 Merge pull request #13 from Gonanf/fe/6-school-integration`
- **Archivos (aprox):** 180
- **Stack detectado:** Lenguajes principales: .go (48 archivos), .html (32 archivos), .astro (19 archivos)

## Stack

- Lenguajes principales: .go (48 archivos), .html (32 archivos), .astro (19 archivos)

## Estructura

```
ocicat-bella/
.codegraph/
  .codegraph/codegraph.db
  .codegraph/daemon.log
  .codegraph/source.json
.omo/
  .omo/drafts
  .omo/run-continuation
.prompts/
  .prompts/fe1.md
  .prompts/fe2.md
  .prompts/fe3.md
  .prompts/fe4.md
  .prompts/fe5.md
  .prompts/fe6.md
  .prompts/frontend-batch1.md
  .prompts/ui-login-flows.md
.sisyphus/
  .sisyphus/drafts
CASOS-USO.md
DESIGN.md
```

## Cómo correr

> Instrucciones genéricas según el stack detectado. Ajustar según el repo.

```bash
# instalar deps
bun install   # o npm install / pnpm install

# desarrollo
bun run dev   # o npm run dev

# build
bun run build
```

## Estado

- **Último commit:** `2026-08-23 Merge pull request #13 from Gonanf/fe/6-school-integration`
- **Clonado en:** `/run/media/chaos/terciario/proyectos/ocicat-bella`
- **Exclusiones del lote:** Forks, Workmatch, el-hornero-digital, mali/meli, Sherut (no tocados por consigna)

## Docs

- `docs/overview.md` — descripción extendida y guía rápida (generado en este lote)

## README original (preservado)

> Contenido previo de README.md recortado a 2000 chars para referencia:

```markdown
# Ocicat Bella

Plataforma FOSS self-hosted para escuelas: materiales educativos centralizados,
entregas de trabajos y sandboxes aislados donde los alumnos ejecutan su código.

## Estado

Diseño MVP cerrado (18 mockups, 15 casos de uso). Frontend Astro en desarrollo.
Ver [DESIGN.md](DESIGN.md) (fuente de verdad) y [CASOS-USO.md](CASOS-USO.md).

## Stack

- **Frontend:** Astro 7 + Tailwind v4 + daisyUI v5 (tema dual claro/oscuro)
- **Backend:** Go (binario único) + Turso
- **Sandboxes:** Docker, contenedores efímeros por entrega
- **Auth:** docentes password / alumnos magic link (nombre+email, sin contraseña)

## Modelo de roles

- **Director** (admin): crea docentes, gestiona el código global de escuela.
  Nace del wizard de instalación (`/setup`).
- **Docente**: crea salas, consignas, materiales; ve entregas.
- **Alumno**: se registra con nombre+email (magic link), entra por código de sala,
  sube entregas, corre sandboxes.
- **Invitado**: con el código global de escuela puede VER proyectos y documentos
  públicos de toda la institución (nunca consignas).

## Alcance MVP

1. Salas con código de invitación (rotable por el docente)
2. Materiales con preview inline
3. Consignas configurables (intentos, tardías)
4. Wizard de entrega 3 pasos con sandbox de prueba y checkpoint
5. Sandboxes Docker efímeros con estados visibles
6. Piloto: 1 escuela × 1 docente × 4 semanas (métricas en §13.4)

Fuera del MVP (v2+): federación, composer visual de Dockerfiles, OAuth,
notificaciones, panel admin completo. Ver §13.4 no-builds.

## Desarrollo

```bash
cd frontend && bun install && bun run dev   # http://localhost:4321
```

Backend Go: pendiente (Fase 2 del plan, ver DESIGN.md §14 ritual).

## Licencia

FOSS — ver LICENSE.

```

---
*README generado/mejorado automáticamente el 2026-09-04 con inspección de repo (opencode/agy pattern: lectura de estructura, lenguaje y entrypoints). No se modificó código, solo documentación.*
*Autor original: Gonanf — https://github.com/Gonanf/ocicat-bella*
