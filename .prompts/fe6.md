Estás implementando la FASE 6 (final) de integración frontend de Ocicat Bella (repo actual, Astro+TS en frontend/, src/lib/api.ts existente). Contrato: docs/api-v1.md — escuela §9 y setup wizard §1/CU-12, gestión código global §13.7. El backend PR7 ya está terminado.

OBJETIVO: wire real del panel escuela/director.

TAREAS:
1. Extendé src/lib/api.ts: getSchool(), regenerateGlobalCode(), deactivateGlobalCode(), listTeachers(), createTeacher(payload), resetTeacherCredential(id), disableTeacher(id), enableTeacher(id), importStudentsCsv(classroomId, file), resendMagicLink(userId), impersonate(userId, reason), stopImpersonation(). Tipos School, Teacher, ImportResult.
2. Panel escuela del director (/escuela): info + código global con regenerar/desactivar (§13.7); gestión docentes: listado con estado invited|active + entregas totales, crear docente, reset credential, disable/enable con copy de cuenta desactivada (CU-15).
3. Alta masiva de alumnos por aula (docente): upload CSV (multipart campo `csv`), mostrar resultado 2 pasos — filas ok commiteadas + reporte de filas inválidas; botón reenviar magic link (respuesta genérica anti-enumeración).
4. Impersonación (director): iniciar con reason obligatoria desde vistas de usuario; banner visible "estás viendo como X" + endpoint separado para volver a sesión director.
5. Setup wizard /setup si falta: GET /setup/status → POST /setup/school → POST /setup/admin; si configured:true redirigir al login; manejar 409 setup_already_done.
6. Nav por rol §13.9.x: director ve todo (pasa checks de docente), docente lo suyo.
7. Limpiá mocks huérfanos restantes.

REGLAS:
- Mantener diseño Tailwind+daisyUI. Commits convencionales; no commitear .omo/, dist/, node_modules.
- `cd frontend && bun run build` debe pasar.
Terminá con resumen de archivos creados/modificados.
