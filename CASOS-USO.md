# Ocicat Bella — Casos de uso end-to-end (MVP)

Referencia de pantallas: mockups en `design/mvp-f2/` y base en `design/c8e8/`.
Decisiones: `DESIGN.md` Partes 9 (entregas/wizard), 11 (identidad), 12/13 (scope MVP),
§0.1 (wizard de instalación) + decisiones nuevas 2026-08-22: rol **director**,
**código global de escuela** y wizard `/setup`.

### Roles
- **Invitado:** sin cuenta; ve contenido público.
- **Alumno:** magic link (nombre + email, §13.1).
- **Docente:** credencial propia; crea salas, consignas y sandboxes.
- **Director (admin):** dueño de la instancia. Se crea en el wizard `/setup` (CU-12).
  Gestiona el código global de escuela (CU-13) y las cuentas docentes (CU-15).
  Jerarquía: director → docentes → salas → alumnos.

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
1. Entra a `index.html` (c8e8). Arriba del fold: **"Soy docente"** / **"Tengo un código de sala"**, más el link **Materiales**. La tercera puerta — **"Tengo un código de escuela"** — vive como link secundario bajo el fold inmediato (ver Fricción).
2. Entra a `materiales.html`: biblioteca estilo Drive, cards con preview inline (PDF/video/img) filtradas por materia. Puede **Ver** (página propia `/materials/:id`) y **Descargar**.
3. Entra a `sandboxes.html`: ve los proyectos del curso corriendo en el server de la escuela (estado corriendo/detenido, autor).
4. Intenta actuar como alumno: tocar "Unirse con código", abrir un sandbox o subir material → se le ofrece registrarse (`login.html`).
5. En `login.html` elige **"Soy invitado"**: vuelve a ver contenido educativo, pero sin poder actuar.

### Flujos alternativos
- Tiene el **código global de escuela** → CU-14: entra como visitante con lectura ampliada (toda la escuela), sin registro.
- Quiere ser alumno → CU-2 (registro con magic link).
- Llega directo por link de sala compartido → `unirse.html` → se le pide cuenta antes del código.
- Viene desde el login pensando que "el código alcanza sin registro" → el texto de `login.html` lo corrige para códigos de SALA; los de ESCUELA sí dan lectura sin registro (CU-14).

### Puntos de fricción / decisión
- **Decisión (§11.4):** modo visitante = VER contenido, NO actuar. No se promete acceso a salas con código sin registro. El código global no rompe esto: solo amplía QUÉ se ve, nunca permite actuar.
- **Landing con 3 acciones:** riesgo de saturar el fold. Decisión: el fold mantiene las dos acciones principales ("Soy docente" / "Tengo un código"); "Tengo un código de escuela" es un link secundario discreto debajo, con microcopy diferenciador ("para recorrer toda la escuela"). Si el test guerrilla (§13.5) muestra confusión entre código de sala vs. de escuela, se evalúa un solo campo que detecta el tipo de código.
- Fricción típica: confusión "invitado ≈ alumno con alias". El copy debe decir qué puede y qué no puede hacer.

---

## CU-2 — INVITADO: registro como alumno

**Actor:** visitante que quiere ser alumno.
**Precondiciones:** tener email.

1. Desde cualquier límite (CU-1 paso 4) llega a `login.html`, pestaña **Crear cuenta**.
2. Ingresa **nombre + email**. Sin contraseña: recibe un **magic link** al email y entra (decisión final §13.1).
3. Aterriza en `mis-salas.html` vacío ("Todavía no pertenecés a ninguna sala").

**Alternativos:** email mal tipeado → reenvío de magic link; recovery manual por el docente desde su panel.
**Decisión:** sin passwords para alumnos (§13.1); OAuth Google diferido post-MVP (§13.4).

---

## CU-3 — ALUMNO: entrar a una sala con código

**Actor:** alumno con cuenta.
**Precondiciones:** cuenta creada (CU-2); código de sala provisto por el docente (ej. `PROG5A`).

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

1. `mis-salas.html`: lista sus salas con pendientes ("3" o "Al día").
2. Abandonar: menú ⋮ del header del card (fuera del botón Entrar, §11.4) → modal con consecuencia explícita: *"Tus entregas quedan archivadas: el docente todavía puede verlas…"* → confirmar.
3. `perfil.html`: nombre, email (es su identificador), sesiones activas + "Cerrar otras sesiones".

**Decisión:** abandono nunca junto a Entrar (riesgo de click accidental); modal obligatorio.

---

## CU-7 — DOCENTE: login y dashboard

**Actor:** docente (credencial propia §13.1). La cuenta docente la crea el **director**
desde su panel admin (CU-15) — ya no se auto-registra.
**Precondiciones:** cuenta docente creada por el director.

1. Login → `dashboard.html`: resumen de sus salas (alumnos, consignas, sandboxes prendidos), botón **"+ Nueva sala"**.
2. Entra a una sala → tabs Consignas / Alumnos / Configuración.

**Nota de jerarquía:** director → crea docentes (CU-15); docente → crea salas (CU-8);
alumno → entra por código de sala (CU-3). El director NO interviene en el día a día
de salas/consignas; si quiere una, se da de alta también como docente.

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

1. Tab Consignas (`consignas.html`) → **"+ Nueva consigna"** → `consigna-nueva.html`.
2. Carga: título, instrucciones, adjuntos opcionales (máx. 10 MB/archivo), **runtime esperado** (Python/Web/C++/Arduino), fecha límite opcional.
3. Config de entrega: **intentos** (ilimitados default / N / uno solo) y **tardías** (permitidas / cerradas). Modificable en cualquier momento, incluso con entregas existentes (§9.1).
4. Publica: los alumnos la ven al instante; el listado muestra métricas por consigna (OK / con error / tardía / sin entregar, ej. "24/28 entregas") → **Ver entregas**.

**Labels en castellano docente:** "entorno del ejercicio" (no Dockerfile), "qué pueden usar los alumnos" (no templates/restricciones) (§13.3).

---

## CU-10 — DOCENTE: materiales, alumnos y rotación de código

1. **Subir materiales** a la biblioteca de la sala (PDF/docs/videos/img) → preview inline para los alumnos (CU-4).
2. **Gestionar alumnos** (`alumnos.html`): ve nombre real + email de cada uno; vetos; recovery de acceso de un alumno (reenvío/reset manual de magic link).
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
5. Sale del panel; su rol no interviene en salas ni consignas.

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
1. Desde `index.html`, link secundario **"Tengo un código de escuela"** → `unirse.html` (o pantalla dedicada) con un solo campo: el código.
2. Código válido → sesión de invitado ampliada (sin cuenta, sin email). Aterriza en una vista "Escuela <nombre>": proyectos (sandboxes) y documentos **públicos de TODAS las salas**, no solo los de una.
3. Puede **Ver** y **Descargar** documentos públicos y ver sandboxes con su estado/autor; puede abrir el detalle y ver logs/artifact de registros históricos (lectura).
4. Intenta actuar (subir, entrar a una sala, abrir una consigna) → límite claro: *las consignas son privadas del curso; para participar, registrate como alumno (CU-2) o pedí un código de sala.*

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

## Resumen de invariantes transversales

- Alumno sin password: magic link nombre+email (§13.1).
- Código de sala = invitación, no identidad (§11.2).
- **Código global de escuela = UNO por instancia, solo el director lo gestiona; da lectura pública de toda la escuela, nunca consignas ni escritura.**
- **Jerarquía: director → docentes → salas → alumnos/invitados.**
- Checkpoint explícito antes de entregar, con número de intento.
- Castellano docente, cero jerga Docker en labels visibles.
- <300 KB por página; funciona en PCs viejas y red escolar.
