Estás implementando la FASE 4 de integración frontend de Ocicat Bella (repo actual, Astro+TS en frontend/, src/lib/api.ts existente). Contrato: docs/api-v1.md §4 (Consignas), §5 (Entregas — wizard 3 pasos: consigna→archivos→probar(opcional, fase FE5)→checkpoint explícito→entregado).

OBJETIVO: wire real de consignas y entregas.

TAREAS:
1. Extendé src/lib/api.ts: createAssignment(classroomId, payload), listAssignments(classroomId), getAssignment, patchAssignment, deleteAssignment, assignmentStats(id), uploadAttachment(file), uploadSubmissionFiles(assignmentId, files[]), deliverSubmission(submissionId, confirmAttempt), mySubmissions(assignmentId), getSubmission(id), listSubmissionsFiltered(assignmentId, filter). Tipos Assignment, AttemptsConfig (unlimited|limited{max}|one), Submission, Stats.
2. consignas.astro (vista docente): listar consignas del aula reales + métricas del listado; consigna-nueva.astro: crear con runtime select, due_at, attempts mode, late_policy; adjuntos subiendo archivo primero (POST /assignments/attachments) y referenciando attachment_ids.
3. Vista alumno de consignas activas con vencimientos (mis-salas → aula).
4. entregas/[id].astro (wizard del alumno): subir archivos (multipart multi), mostrar intento en curso N de max, checkpoint explícito "¿Entregar? (intento N de M)" → deliverSubmission con confirm_attempt; manejar 409 attempt_conflict (refrescar), attempts_exhausted, deadline_passed con copy del contrato; flags tardía/tested_ok cuando vengan; historial /me.
5. Vista corrección docente (entregas por consigna): GET submissions?filter=late|missing|error + stats endpoint.
6. Limpiá de mock.ts lo que quede huérfano (consignas mock).

REGLAS:
- Mantener diseño Tailwind+daisyUI. Commits convencionales; no commitear .omo/, dist/, node_modules.
- `cd frontend && bun run build` debe pasar.
Terminá con resumen de archivos creados/modificados.
