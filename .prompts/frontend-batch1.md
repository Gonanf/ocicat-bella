# Tarea: implementar las pantallas del MVP de Ocicat Bella en Astro

Sos el Frontend Developer del equipo Ocicat Bella. Trabajá en
/home/chaos/proyectos/ocicat-bella/frontend (Astro 7.2.4 + Tailwind v4 +
daisyUI v5, bun, tema dual ocicat/ocicat-dark funcionando).

## Referencias obligatorias (leé primero)
- DESIGN.md del proyecto (Partes 9, 11, 13 — decisiones vigentes)
- CASOS-USO.md (flujos por rol)
- Mockups HTML: ../design/mvp-f2/*.html (16 pantallas — son la referencia visual)
- Frontend existente: src/pages/, src/layouts/Layout.astro, src/data/mock.ts,
  src/styles/global.css

## Tu trabajo esta sesión (batch 1 — pantallas alumno)
Implementa en Astro estas 5 páginas nuevas, siguiendo los mockups como guía
visual exacta y usando el tema daisyUI ocicat existente (clases semánticas
bg-base-100/text-base-content/etc — NADA de colores hex hardcodeados):
1. /login — tabs Entrar/Crear cuenta; magic link (nombre+email, sin password);
   link secundario "código de escuela" para visitante.
2. /mis-salas — cards de salas del alumno + abandonar vía menú 3 puntos con
   modal de confirmación.
3. /perfil — datos, cambiar magic link email, sesiones activas.
4. /entregas/[id] — wizard de entrega 3 pasos (consigna → archivos+entorno →
   probar y entregar) con checkpoint "¿Entregar? (intento N de M)".
5. /consignas — lista de consignas de la sala para el alumno.

Datos: extende src/data/mock.ts con mocks coherentes (salas PROG5A/ROBO6B ya
existen). No toques backend ni config de build. NO hagas commit — dejá los
cambios en el working tree.

## Verificación
`bun run build` debe terminar exit 0 con todas las páginas. Reportá: páginas
creadas, archivos tocados, resultado del build.
