# Ocicat Bella — Casos de uso end-to-end (MVP)

Referencia de pantallas: mockups en `design/mvp-f2/` y base en `design/c8e8/`.
Decisiones: `DESIGN.md` Partes 9 (entregas/wizard), 11 (identidad), 12/13 (scope MVP),
§0.1 (wizard de instalación) + decisiones 2026-08-22: rol **director**,
**código global de escuela**, wizard `/setup` y **§13.9.x** — login unificado
(email → detección de rol), QR inverso para PCs compartidas, magic link como
fallback, **alta de alumnos cerrada** y navegación por rol con labels ("Aulas",
notificaciones con badge).

### Roles
- **Invitado:** sin cuenta; ve contenido público (con o sin código global).
- **Alumno:** SIN password y SIN registro self-service. Lo da de alta un
  docente/admin (CU-18). Empareja su celular una vez vía magic link (CU-17);
  después entra en PCs compartidas por **QR inverso** (CU-16) o, si no tiene
  celular, por magic link desde la misma PC (CU-2).
- **Docente:** credencial propia (password); la crea el director (CU-15).
  Crea aulas y da de alta alumnos.
- **Director (admin):** dueño de la instancia. Se crea en el wizard `/setup`
  (CU-12). Gestiona el código global (CU-13), cuentas docentes (CU-15) y el
  alta de alumnos (CU-18). **Ve todo y gestiona sin necesitar credenciales de
  docente** (§13.9.2). Jerarquía: director → docentes → aulas → alumnos.

### Navegación por estado y rol (§13.9.x)
- **Anónimo:** CTA **"Ingresar con código de escuela"** + link **"Iniciar sesión"**.
  Sin botón "Admin" visible (el acceso institucional es un link discreto, §13.9.1).
- **Invitado:** Materiales · Proyectos.
- **Registrado:** **Aulas** + Materiales + Proyectos + Tareas/Consignas según rol
  + **notificaciones (campana con badge)** + Perfil/logout. El director agrega
  **Gestión Escolar**. Microcopy oficial: **"Aulas"**, nunca "Salas".
  La creación de materiales/consignas vive DENTRO del aula, no en la nav global.

### Código global de escuela vs código de sala
- **Código global:** UNO por instancia, gestionado solo por el director. Da a
  invitados acceso de LECTURA a proyectos y documentos públicos de TODA la escuela.
  NO da acceso a consignas ni permite actuar.
- **Código de sala:** lo genera cada docente; inscribe al alumno en esa sala (CU-3).

---

## CU-1 — INVITADO: explorar contenido y chocar con el límite

**Actor:** visitante sin cuenta.
**Precondiciones:** ninguna; la instancia es accesible por navegador.

### Flujo principal
1. Entra a `index.html` (c8e8). Nav anónima (§13.9.2): CTA **"Ingresar con código de escuela"** + link **"Iniciar sesión"**. NO hay botón "Admin" ni "Registrarse" — el registro de alumnos está cerrado (CU-18) y el acceso institucional queda como link discreto.
2. Entra a `materiales.html`: biblioteca estilo Drive, cards con preview inline (PDF/video/img) filtradas por materia. Puede **Ver** (página propia `/materials/:id`) y **Descargar**.
3. Entra a `sandboxes.html`: ve los proyectos del curso corriendo en el server de la escuela (estado corriendo/detenido, autor).
4. Intenta actuar (entrar a un aula, abrir una consigna, subir material) → límite claro: se le ofrece **"Iniciar sesión"**; si no tiene cuenta, no puede crearla solo (alta cerrada, CU-18).

### Flujos alternativos
- Tiene el **código global de escuela** → CU-14: entra como visitante con lectura ampliada (toda la escuela), sin registro.
- Quiere ser alumno → no hay registro self-service; pide al docente que lo dé de alta (CU-18).
- Llega directo por link de sala compartido → `unirse.html` → se le pide cuenta antes del código.
- Viene desde el login pensando que "el código alcanza sin registro" → el texto de `login.html` lo corrige para códigos de SALA; los de ESCUELA sí dan lectura sin registro (CU-14).

### Puntos de fricción / decisión
- **Decisión (§11.4):** modo visitante = VER contenido, NO actuar. No se promete acceso a salas con código sin registro. El código global no rompe esto: solo amplía QUÉ se ve, nunca permite actuar.
- **Landing con 3 acciones:** riesgo de saturar el fold. Decisión: el fold mantiene las dos acciones principales ("Soy docente" / "Tengo un código"); "Tengo un código de escuela" es un link secundario discreto debajo, con microcopy diferenciador ("para recorrer toda la escuela"). Si el test guerrilla (§13.5) muestra confusión entre código de sala vs. de escuela, se evalúa un solo campo que detecta el tipo de código.
- Fricción típica: confusión "invitado ≈ alumno con alias". El copy debe decir qué puede y qué no puede hacer.

---

## CU-2 — ALUMNO SIN CELULAR: login por magic link desde la PC

**Actor:** alumno dado de alta (CU-18) que no tiene celular emparejado (o lo
prefiere así). Es el fallback oficial del QR inverso (§13.9.4).
**Precondiciones:** cuenta creada por docente/admin; acceso a su email desde la
PC del colegio o el celular de un compañero.

1. En la PC compartida elige **"Iniciar sesión"** → escribe **su email**.
2. El sistema detecta rol alumno → envía **magic link** al email (sin password,
   §13.1). El alumno abre el link (desde webmail en la misma PC o desde el
   celu prestado) y la PC queda con su sesión.
3. Si todavía no tenía email/cuenta: el docente puede crearle la cuenta al
   momento durante el alta (CU-18); el alumno nunca se registra solo.

**Alternativos:** email mal tipeado → reenvío de magic link; recovery manual por
el docente desde su panel (CU-10). Email no llega (red escolar bloquea SMTP) →
usar QR inverso con celular propio o pedir ayuda al docente.

**Decisión:** este flujo es el PLAN B; el camino default en PC compartida es el
QR inverso (CU-16). OAuth Google diferido post-MVP (§13.4).

---

## CU-3 — ALUMNO: entrar a una sala con código

**Actor:** alumno con cuenta.
**Precondiciones:** cuenta dada de alta por docente/admin (CU-18) y sesión
iniciada en el dispositivo (QR inverso CU-16, magic link CU-2, o celu
emparejado CU-17); código de sala provisto por el docente (ej. `PROG5A`).

1. Desde `mis-salas.html` toca "Unirme con código" → `unirse.html`.
2. Escribe el código (6–8 caracteres). Validación inline si está mal.
3. Código válido → queda inscripto; la sala aparece en `mis-salas.html` con consignas pendientes y docente visible.

**Alternativos:**
- Sin cuenta primero: mini-registro → código (§11.2).
- Código rotado/vencido → error claro + pedir nuevo código al docente.

**Fricción:** el código es *invitación*, no identidad (§11.2). Métrica clave de test: % que entra sin ayuda (§13.5).

---

## CU-4 — ALUMNO: ver materiales de la sala

**Actor:** alumno inscripto.
**Precondiciones:** pertenece a la sala (CU-3).

1. Entra a la sala desde `mis-salas.html` → sección Materiales.
2. Cards con preview inline; click abre página propia full-width con botón descargar (§9.3).

**Alternativo:** archivo Office → card con metadata + "Abrir" (sin preview embebido).

---

## CU-5 — ALUMNO: wizard de entrega (núcleo del MVP)

**Actor:** alumno con consigna pendiente.
**Precondiciones:** consigna activa en su sala; archivos de la solución.

1. En `mis-salas.html` entra a la sala y elige la consigna (ej. TP2) → `entregar.html`. Wizard de **3 pasos** (fusión §13.2):
   - **Paso 1 — Consigna:** lee título, instrucciones y adjuntos.
   - **Paso 2 — Archivos:** drag & drop (.zip, .py, index.html…), preview inline. Sin editor embebido (§9.2).
   - **Paso 3 — Probar (opcional):** lanza un sandbox efímero modo job y ve el output antes de entregar (estados del CU-7). Si saltea la prueba, sigue directo.
   - **Paso 4 — Entregar:** **checkpoint explícito**: "¿Entregar? (intento 2 de 3)" → confirma → pantalla **"Entregado ✓ a las HH:MM"**.
2. La entrega guarda snapshot del código + resultado de la prueba; el docente la ve con estado (entregada / probada OK / error / tardía) (§9.2).

**Alternativos:**
- Reintento según config: intentos ilimitados (default), N o uno solo (§9.1).
- Fuera de fecha: entregas tardías permitidas (marcadas) o cerradas al vencimiento, según config.
- Sandbox falla o no hay capacidad → plan B sin Docker: entrega de archivos igual funciona (§13.4).

**Friction points:** checkpoint evita entregas accidentales; contador de intentos visible; feedback <2s en cada acción (PCs viejas).

---

## CU-6 — ALUMNO: mis salas / abandonar sala

**Actor:** alumno.
**Precondiciones:** sesión activa.

1. `mis-salas.html` (label de nav: **Mis Aulas**): lista sus aulas con pendientes ("3" o "Al día").
2. Abandonar: menú ⋮ del header del card (fuera del botón Entrar, §11.4) → modal con consecuencia explícita: *"Tus entregas quedan archivadas: el docente todavía puede verlas…"* → confirmar.
3. `perfil.html`: nombre, email (es su identificador), sesiones activas + "Cerrar otras sesiones".

**Decisión:** abandono nunca junto a Entrar (riesgo de click accidental); modal obligatorio.

---

## CU-7 — DOCENTE: login y dashboard

**Actor:** docente (credencial propia §13.1). La cuenta docente la crea el **director**
desde su panel admin (CU-15) — ya no se auto-registra.
**Precondiciones:** cuenta docente creada por el director.

1. En la nav anónima toca **"Iniciar sesión"** → escribe su email → el sistema
   detecta rol no-alumno → pasa a una segunda pantalla de **password** (login
   unificado, §13.9.2). Sin detour hacia pantallas separadas por rol.
2. Login → `dashboard.html`: resumen de sus aulas (alumnos, consignas,
   sandboxes prendidos), botón **"+ Nueva aula"**.
3. Entra a un aula → tabs Consignas / Alumnos / Configuración. Desde acá crea
   materiales y consignas (botones en la página del aula, §13.9.2) — NO hay
   ítems globales de creación en la nav.
4. Nav registrada (rol docente): **Aulas** + Materiales + Proyectos + Consignas
   + campana de notificaciones con badge + Perfil/logout.

**Nota de jerarquía:** director → crea docentes (CU-15); docente → crea aulas
(CU-8) y da de alta alumnos (CU-18); alumno → entra por código de aula (CU-3).
El director NO necesita credenciales de docente para ver/gestionar (§13.9.2);
si quiere trabajar el día a día de un aula, actúa con su rol admin que ve todo.

---

## CU-8 — DOCENTE: crear sala y compartir código

1. Dashboard → "+ Nueva sala": nombre, curso, turno.
2. Sala creada → código visible (ej. `PROG5A`) con **Copiar**; comparte el código con la clase.
3. Los alumnos se suman solos vía CU-3; aparecen en tab **Alumnos** (`alumnos.html`: inscriptos, entregas totales).

**Alternativo:** fuga del código fuera del aula → rotar código (CU-10).

---

## CU-9 — DOCENTE: crear consigna con config de entrega

**Actor:** docente de la sala.
**Precondiciones:** sala creada (CU-8).

1. Tab Consignas **dentro del aula** (`consignas.html`) → botón **"+ Nueva consigna"** (vive en la página del aula, no en la nav global, §13.9.2) → `consigna-nueva.html`.
2. Carga: título, instrucciones, adjuntos opcionales (máx. 10 MB/archivo), **runtime esperado** (Python/Web/C++/Arduino), fecha límite opcional.
3. Config de entrega: **intentos** (ilimitados default / N / uno solo) y **tardías** (permitidas / cerradas). Modificable en cualquier momento, incluso con entregas existentes (§9.1).
4. Publica: los alumnos la ven al instante; el listado muestra métricas por consigna (OK / con error / tardía / sin entregar, ej. "24/28 entregas") → **Ver entregas**.

**Labels en castellano docente:** "entorno del ejercicio" (no Dockerfile), "qué pueden usar los alumnos" (no templates/restricciones) (§13.3).

---

## CU-10 — DOCENTE: materiales, alumnos y rotación de código

1. **Subir materiales** a la biblioteca del aula (botón en la página del aula, no item global de nav — §13.9.2) → preview inline para los alumnos (CU-4).
2. **Gestionar alumnos** (`alumnos.html`): ve nombre real + email de cada uno; vetos; recovery de acceso de un alumno (reenvío manual de magic link); **alta de alumnos nuevos** (CU-18).
3. **Rotar código**: botón visible con confirmación y aviso *"se genera uno nuevo y el actual deja de funcionar al instante"* (§13.3 — no escondido en menú).
4. **Configuración de sala** (`sala-config.html`): templates permitidos y herramientas visibles (defaults sensatos hardcoded; restricciones granulares diferidas a v2, §13.4). Toggle "Dockerfile propio" default OFF (§10.5).

**Fricción:** rotar saca acceso a toda la clase → doble aviso antes de confirmar.

---

## CU-11 — TRANSVERSAL: lanzar y seguir un sandbox

**Actor:** alumno (y docente para probar).
**Precondiciones:** sala con templates habilitados.

1. Desde la sala o el wizard (CU-5 paso 3): elegir template → `nuevo-sandbox.html`. Modo **Job** (corre y se apaga) o **Service** (queda prendido mientras trabajás). El 90% solo elige template y corre (§10.2).
2. **Modo avanzado (oculto por defecto):** lenguaje + campo "paquetes extra" en texto plano es el camino default; el composer visual completo quedó diferido a v2 (§13.4). Dockerfile propio solo si el docente lo habilitó, con lint y build aislados en backend (sin UI propia, §13.4/§10.4).
3. **Estados intermedios visibles** (§13.2): *encolando → descargando imagen → arrancando → listo*, con feedback <2s por acción. Presupuesto de contenedores: si el server está lleno, espera de turno visible ("2 de 4 sandboxes prendidos").
4. **Ver resultados:** service Web = iframe al puerto; job/consola = terminal de logs en vivo (`sandbox-detalle.html`: exit code, historial de ejecuciones #N exitosa/falló/timeout); Arduino = salida serial simulada.
5. Imágenes derivadas se cachean (dedupe §6.2) → arranques cada vez más rápidos para la clase.

**Puntos de decisión:**
- Plan B sin Docker: la entrega de archivos funciona aunque fallen sandboxes (riesgo #1 mitigado, §13.4).
- Labels sin jerga Docker hacia el alumno/docente; la jerga queda interna.

---

## CU-12 — DIRECTOR: wizard de instalación primera vez (/setup)

**Actor:** director de la escuela (quien instala el binario; puede no saber terminal).
**Precondiciones:** binario instalado (instalador .exe/.msi en Windows, o copiado en el server); arranca y abre `http://localhost:8484/setup` (§0.1). Pantalla nueva: `setup.html` (mockup pendiente).

### Flujo principal
1. **Bienvenida:** pantalla de `setup.html` que explica en una línea qué es Ocicat y qué se va a configurar. Sin jerga técnica.
2. **Nombre de la escuela:** un campo ("¿Cómo se llama tu escuela?"). Es el nombre que ven todos los usuarios y las escuelas federadas.
3. **Cuenta del director:** nombre + email + credencial propia (es UN usuario; password permitida, §13.1). Queda como rol **director**, dueño de la instancia.
4. **Código global de escuela generado automáticamente:** pantalla de éxito que muestra el código con **Copiar** y una explicación llana de qué da acceso: *"Quien tenga este código puede VER los proyectos y documentos públicos de toda la escuela, sin cuenta. No ve consignas ni puede hacer cambios."* (CU-13/CU-14).
5. **"Ir al dashboard"** → entra al panel admin (`admin.html`, CU-13) ya autenticado.

### Flujos alternativos
- Wizard interrumpido a mitad → al reabrir `/setup` retoma en el paso donde quedó; la instancia no queda usable hasta terminar.
- Puerto 8484 ocupado → el wizard avisa y propone otro puerto (caso raro; copy simple).
- Ya existe una instalación (re-run del binario) → `/setup` redirige al login con aviso "esta escuela ya está configurada".

### Puntos de fricción / decisión
- Objetivo §0.1: que cualquier profe lo levante **sin tocar la terminal**. El wizard es el único onboarding; el CLI queda para el SaaS headless.
- El código global se genera solo (cero decisiones extra en el wizard); regenerarlo es tarea posterior del panel admin (CU-13).
- Fricción: que el instalador no abra el navegador solo (Windows raro) → pantalla de fallback con la URL para copiar.

---

## CU-13 — DIRECTOR: gestionar el código global de escuela

**Actor:** director (único rol con acceso).
**Precondiciones:** instancia configurada (CU-12). Pantalla nueva: `admin.html` (mockup pendiente).

### Flujo principal
1. Login como director → `admin.html` (panel admin). Secciones: **Código de escuela**, **Docentes** (CU-15), datos de la escuela.
2. Ve el código global con **Copiar** y, al lado, el texto de qué habilita: lectura de proyectos y documentos **públicos de toda la escuela** para quien lo tenga, sin registro. Explícito: **no** da consignas ni escritura.
3. Comparte el código (ej. en la web de la escuela, cartel de exposición, jornada de puertas abiertas).
4. **Regenerar:** botón visible con confirmación y aviso *"el código actual deja de funcionar al instante; quien lo tenga pierde el acceso hasta que le des el nuevo"* (mismo patrón que rotar código de sala, CU-10).
5. Sale del panel. Con la decisión §13.9.2, el director **ve todo y gestiona
   sin credenciales de docente** (acceso directo por rol o impersonación
   administrativa, a definir en backend) — ya no hace falta que se dé de alta
   también como docente para intervenir en un aula.

### Flujos alternativos
- Sospecha de fuga del código (circuló donde no quería) → regenera; los invitados viejos quedan afuera.
- Quiere desactivar el acceso por código global sin borrar nada → botón "Desactivar código" (estado: sin código vigente; se puede generar uno nuevo cuando quiera).

### Puntos de fricción / decisión
- **Decisión:** el código global es UNO por instancia y SOLO el director lo gestiona. Un docente no puede regenerarlo (evita que un docente de a baja el acceso de toda la escuela sin saberlo).
- Fricción: confusión con el código de sala. El copy del panel distingue siempre: *"código de escuela (visitas)"* vs *"códigos de sala (alumnos)"*.

---

## CU-14 — INVITADO: entrar con el código global de escuela

**Actor:** visitante con el código global (familiares, jurado de feria de ciencias, escuela visitante, vecino curioso).
**Precondiciones:** tener el código global vigente; instancia accesible.

### Flujo principal
1. Desde `index.html`, CTA principal **"Ingresar con código de escuela"** (§13.9.2) → `unirse.html` (o pantalla dedicada) con un solo campo: el código.
2. Código válido → sesión de invitado ampliada (sin cuenta, sin email). Nav pasa a estado **invitado**: Materiales · Proyectos. Aterriza en una vista "Escuela <nombre>": proyectos (sandboxes) y documentos **públicos de TODAS las salas**, no solo los de una.
3. Puede **Ver** y **Descargar** documentos públicos y ver sandboxes con su estado/autor; puede abrir el detalle y ver logs/artifact de registros históricos (lectura).
4. Intenta actuar (subir, entrar a una sala, abrir una consigna) → límite claro: *las consignas son privadas del curso; para participar necesitás que tu docente te dé de alta (CU-18) o pedí un código de sala.*

### Flujos alternativos
- Código inválido o regenerado → error claro: *"este código ya no funciona; pedile el nuevo a la escuela"*.
- Llega por link `ocicat.edu/s/CODIGO` compartido → mismo flujo, sin pasar por la landing.
- Ya tiene cuenta de alumno → el código global no le agrega nada (ya ve más); el copy lo aclara para no generar doble identidad.

### Puntos de fricción / decisión
- **Decisión:** el código global es la alternativa al registro para visitantes: da LECTURA de todo lo público de la escuela, nunca consignas ni acciones (§11.4 se mantiene: ver, no actuar).
- Fricción principal: expectativa de "ver todo". Copy explícito sobre qué queda privado (consignas, entregas, materiales marcados privados por el docente).
- Métrica: % de invitados con código que llegan a ver al menos un proyecto (vs. abandonan en el campo de código).

---

## CU-15 — DIRECTOR: gestión básica de docentes

**Actor:** director.
**Precondiciones:** panel admin (`admin.html`, CU-13). MVP: crear y desactivar; edición granular diferida (§13.4 — sin panel admin elaborado).

### Flujo principal
1. `admin.html` → sección **Docentes** → **"+ Crear cuenta docente"**.
2. Carga: nombre + email + credencial inicial (o magic link de primer acceso, coherente con §13.1). El docente aparece en la lista con estado **activo**.
3. El director le pasa las credenciales/enlace al docente; este hace login y arranca en CU-7.
4. **Desactivar** un docente (menú ⋮ de la fila): modal con consecuencia explícita — *"no podrá entrar; sus salas y consignas quedan intactas"* → confirmar.

### Flujos alternativos
- Docente olvidó credencial → el director la resetea desde la misma fila (reenvío de magic link / nueva credencial), mismo patrón que recovery de alumnos (CU-10).
- Reactivar un docente desactivado → toggle en la fila; vuelve con sus salas como estaban.
- Docente desactivado intenta entrar → error claro que lo deriva al director, no a un loop de login.

### Puntos de fricción / decisión
- **Decisión:** las cuentas docentes ya no se auto-registran; el director es la puerta. Cierra el modelo: nadie entra a la instancia sin que el director lo habilite (docente) o tenga un código (alumno/invitado).
- Qué pasa con las salas del docente desactivado: quedan congeladas (lectura para alumnos, sin consignas nuevas) — decisión mínima MVP; reasignación de salas a otro docente queda para v2.
- Fricción: escuela grande con un solo director. Aceptado para MVP (piloto 1 escuela × 1 docente, §13.4).

---

## CU-16 — ALUMNO: login en PC compartida por QR inverso

**Actor:** alumno con celular ya emparejado (CU-17). Flujo default en el
laboratorio de PCs (§13.9.3/§13.9.4).
**Precondiciones:** cuenta activa (CU-18); PWA instalada/abierta en su celular
y emparejada; PC del colegio con la instancia accesible.

### Flujo principal
1. En la PC compartida toca **"Iniciar sesión"** → pantalla **"Escanear QR"**:
   muestra un **QR efímero** (single-use, TTL ≤90 s, regeneración automática
   ~60 s, entropía ≥128 bits, URL del dominio oficial — nunca shortener,
   §13.9.5).
2. El alumno abre la PWA en su celular → "Escanear QR" → apunta a la PC.
3. **Confirmación en el celular:** muestra la identidad de la PC — *"¿Conectar
   con LAB-PC07?"* → el alumno confirma.
4. La PC queda con SU sesión: aterriza en Mis Aulas (`mis-salas.html`).
5. **Sesión temporal:** TTL ≤60 min e inactividad ≤10 min con aviso previo;
   cookies HttpOnly/Secure/SameSite=Strict; cero persistencia al cerrar el
   navegador (§13.9.5). Al expirar vuelve al QR.

### Flujos alternativos
- QR vencido → se regenera solo; si el alumno escanea uno viejo → error claro
  ("este código ya no sirve, mirá el nuevo").
- Rechaza la conexión en el celu → la PC nunca inicia sesión (claim atómico,
  sin doble claim).
- **Sin celular o sin datos:** fallback magic link desde la misma PC (CU-2),
  con email propio o creado por el docente al momento del alta (CU-18).

### Puntos de fricción / decisión
- Descartado el QR/código impreso por alumno (850 alumnos no escalan, §13.9.3).
- Auditoría de seguridad APROBADA CON CONDICIONES (§13.9.5): las condiciones 3
  y 4 de arriba son obligatorias antes de producción.
- Fricción esperada: alumno deja la PC con sesión abierta → mitigado por
  expiración corta + aviso de inactividad.

---

## CU-17 — ALUMNO: emparejamiento inicial del celular vía magic link

**Actor:** alumno dado de alta (CU-18), primera vez con su celular personal.
**Precondiciones:** cuenta creada por docente/admin; acceso a su email desde
el celular; límite de ~3 dispositivos por cuenta (§13.9.5).

### Flujo principal
1. Instala/abre la **PWA** de Ocicat en su celular (liviana, <300 KB).
2. Ingresa su **email** → recibe **magic link** validado por email (única vía
   de pairing inicial, condición 2 de §13.9.5).
3. Abre el link EN EL CELULAR → el dispositivo queda emparejado con su cuenta.
4. Desde ese momento, en cualquier PC del colegio entra por QR inverso (CU-16)
   sin volver a tocar el email.

### Flujos alternativos
- Email mal tipeado o no llega → reenvío; recovery manual por el docente/admin.
- Cambia/pierde el celular → empareja el nuevo (cuenta el límite de
  dispositivos); puede desvincular desde `perfil.html`.
- Email escolar compartido (hermanos, casilla de curso) → riesgo real
  documentado como requisito de despliegue: preferir casillas individuales
  (§13.9.5 nota final).

---

## CU-18 — DOCENTE/DIRECTOR: alta de alumnos (registro cerrado)

**Actor:** docente del aula (o director/admin).
**Precondiciones:** aula creada (CU-8); tab Alumnos (`alumnos.html`).
**Decisión clave (§13.9.5, condición 1): NO existe registro self-service de
alumnos. La puerta es el docente/admin — decisión de arquitectura más
importante de la auditoría de seguridad.**

### Flujo principal
1. `alumnos.html` → botón **"+ Dar de alta alumnos"**.
2. Dos caminos:
   - **Import de lista CSV:** sube el listado del curso (nombre + email) →
     preview de filas → confirmar. Emails duplicados o inválidos se rechazan
     fila por fila con motivo claro.
   - **Invitación individual:** carga nombre + email de un alumno → alta al
     momento (útil para quien se suma a mitad de año).
3. Cada alumno queda con cuenta activa SIN password; el sistema le envía (o el
   docente le pasa) el magic link de primer acceso → emparejamiento (CU-17) o
   login directo (CU-2).
4. El alumno aparece en el listado del aula con estado (invitado pendiente /
   activo).

### Flujos alternativos
- Baja de un alumno → mismo menú que vetas/desactivar; sus entregas quedan
  archivadas (mismo patrón que abandono de sala, CU-6/CU-10).
- Director da de alta desde Gestión Escolar con acceso directo por rol, sin
  credenciales de docente (§13.9.2).
- Data minimization: solo nombre/curso/email, nada más; borrado real de cuenta
  disponible (condición 6, §13.9.5).

### Puntos de fricción / decisión
- Fricción aceptada: el alumno NO puede empezar solo — necesita al docente.
  Es el precio explícito de cerrar el vector de abuso de cuentas anónimas.
- CSV con casillas compartidas → advertencia en la preview (requisito de
  despliegue, §13.9.5).

---

## Resumen de invariantes transversales

- Alumno SIN password y SIN registro self-service: alta solo por docente/admin (CU-18).
- Login del alumno en PC compartida = QR inverso (CU-16); magic link es fallback (CU-2); pairing inicial solo vía magic link validado (CU-17).
- Sesiones de PC de alumno: temporales (≤60 min / inactividad 10 min), sin persistencia.
- Login unificado: email → detección de rol → alumno magic link, docente/director password. Sin botón "Admin" en nav anónima.
- Nav por estados con labels por rol; microcopy **"Aulas"** (nunca "Salas"); notificaciones con badge entran al MVP.
- Creación de materiales/consignas DENTRO del aula, nunca como item global de nav.
- Director ve todo y gestiona sin credenciales de docente.
- Código de sala = invitación, no identidad (§11.2).
- **Código global de escuela = UNO por instancia, solo el director lo gestiona; da lectura pública de toda la escuela, nunca consignas ni escritura.**
- **Jerarquía: director → docentes → salas → alumnos/invitados.**
- Checkpoint explícito antes de entregar, con número de intento.
- Castellano docente, cero jerga Docker en labels visibles.
- <300 KB por página; funciona en PCs viejas y red escolar.
