Estás implementando la FASE 3 de integración frontend de Ocicat Bella (repo actual, Astro+TS en frontend/, ya existe src/lib/api.ts con ApiError). Contrato: docs/api-v1.md §3 (Aulas), §6 (Materiales), §10 (Invitados/código global).

OBJETIVO: wire real de aulas, materiales y flujo de invitado, reemplazando mocks.

TAREAS:
1. Extendé src/lib/api.ts: createClassroom, listClassrooms (scope por rol del contrato), getClassroom, patchClassroom, deleteClassroom, getJoinCode, rotateJoinCode, joinClassroom(code), leaveClassroom, listStudents(classroomId), removeStudent; materials: listMaterials(scope/subject), uploadMaterial(multipart file,title,visibility,subject), getMaterial, getMaterialFileUrl (stream/download=1), patchMaterial, deleteMaterial; school: getSchool, regenerateGlobalCode, disableGlobalCode, guestSession(code), schoolPublic. Tipos Classroom, Material, StudentRow, SchoolInfo.
2. mis-salas.astro: aulas reales por rol — alumno ve sus aulas con pending_assignments/up_to_date; docente stats; director todas + docente. Botón abandonar (DELETE membership/me) con confirmación.
3. unirse.astro: POST /classrooms/join con manejo 404 invalid_code ("este código no funciona; pedile el nuevo a tu docente") y 409 already_member.
4. dashboard.astro (docente): crear aula real (POST /classrooms), ver join_code, rotar código (con doble confirmación UI antes de confirmar, aviso que el anterior muere al instante), listar alumnos del aula (alumnos.astro) con baja (DELETE students/{id}).
5. materiales.astro: listado según quien pregunta (?scope=), subida dentro del aula con visibility, descarga ?download=1, borrar si es autor/director.
6. index.astro o vista pública de escuela: invitado ingresa código global → POST /guest/sessions → vista "Escuela <nombre>" con materiales public/school; mutación como invitado debe mostrar el copy 403 guest_read_only.
7. sala-config.astro: settings de aula (allowed_templates etc.) — dejá el shell listo pero la data llega en fase FE5/sandboxes; solo wire básico si aplica.
8. Limpiá de mock.ts lo que quede huérfano.

REGLAS:
- Mantener diseño Tailwind+daisyUI. Commits convencionales; no commitear .omo/, dist/, node_modules.
- `cd frontend && bun run build` debe pasar.
Terminá con resumen de archivos creados/modificados.
