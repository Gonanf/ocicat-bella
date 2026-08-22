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
