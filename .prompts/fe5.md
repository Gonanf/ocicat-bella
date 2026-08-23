Estás implementando la FASE 5 de integración frontend de Ocicat Bella (repo actual, Astro+TS en frontend/, src/lib/api.ts existente). Contrato: docs/api-v1.md — sandboxes/runs §7 (presupuesto → 202 cola / 503 sin 429), templates §10.2 y settings de sala G4 (PATCH /classrooms/{id}/settings con allowed_templates + custom_dockerfile_enabled). El backend PR6 ya está terminado.

OBJETIVO: wire real de sandboxes.

TAREAS:
1. Extendé src/lib/api.ts: listTemplates(), getClassroomSettings(id), patchClassroomSettings(id, payload), createSandbox(classroomId, payload), getSandbox(id), stopSandbox(id), logs SSE del sandbox (EventSource), listRuns(...). Tipos Template, SandboxSettings, Sandbox/Run (estados del contrato).
2. Sandboxes UI: crear sandbox desde plantilla dentro del aula respetando allowed_templates; manejar 202 (en cola, mostrar posición/estado vía polling o SSE de logs) y 503 presupuesto agotado con copy del contrato (sin tratarlo como error genérico); stop; ephemeral 404 tras expiración.
3. Settings de sala (docente): vista/panel para PATCH settings (allowed_templates checkboxes + toggle custom Dockerfile).
4. Consola de logs en vivo via SSE donde corresponda.
5. Limpiá mocks huérfanos de sandboxes de mock.ts.

REGLAS:
- Mantener diseño Tailwind+daisyUI. Commits convencionales; no commitear .omo/, dist/, node_modules.
- `cd frontend && bun run build` debe pasar.
Terminá con resumen de archivos creados/modificados.
