# Consulta de diseño: navegación por rol — Ocicat Bella

Sos el UI Designer del equipo Ocicat Bella. Evaluá esta especificación de
navegación propuesta por el usuario y respondé si está bien o qué ajustarías.

## Contexto del producto
Plataforma educativa self-hosted para escuelas secundarias argentinas. Roles:
Director (admin) > Docente > Alumno > Invitado (acceso lectura con código
global de escuela). Los alumnos usan magic link (sin password); docentes y
director password. Público objetivo: docentes poco técnicos y alumnos de
secundaria con PCs viejas — la nav debe ser simple y obvia.

## Propuesta del usuario (nav por estado)

Sin registro (visitante anónimo):
- Admin (iniciar sesión)
- Login/Registrarse
- Ingresar escuela con código

Invitado (entró con código global de escuela):
- Materiales (todo)
- Proyectos (todo)

Registrado (estudiante, profesor O admin — mismo nav para los tres):
- Perfil + logout
- Salas
- Materiales (todo)
- Proyectos (todo)
- Consignas (solo pendientes)

## Preguntas específicas
1. ¿La nav unificada para estudiante/profesor/admin funciona, o conviene
   diferenciar items según rol dentro del estado "registrado"? Considerá que
   crear materiales/consignas/salas son acciones de docente, no de alumno.
2. ¿Qué hace el item "Salas" para cada uno de los tres roles registrados?
3. ¿Falta algo? ¿Sobra algo?
4. Para el visitante anónimo, ¿"Admin (iniciar sesión)" como item separado del
   "Login/Registrarse" te parece correcto o confunde?

## Formato de respuesta
Veredicto por pregunta (OK / ajustar con propuesta concreta), más cualquier
riesgo UX que veas. Conciso, markdown, español rioplatense.
