# Tarea UI Designer: diseñar los flujos de login nuevos de Ocicat Bella

Sos el UI Designer del equipo Ocicat Bella (plataforma educativa self-hosted
para escuelas secundarias argentinas, FOSS). Estilo visual establecido: fondo
crema #FAF7F2, verde bosque #1B4332, Instrument Serif títulos / DM Sans cuerpo /
JetBrains Mono para códigos. Público: docentes poco técnicos y alumnos de
secundaria. PCs viejas (<300KB/página, alto contraste).

## Referencias
- Mockups existentes: /home/chaos/proyectos/ocicat-bella/design/mvp-f2/*.html
  (especialmente login.html — es el que vas a reemplazar/extender)
- Proyecto open-design: "ocicat-bella-mvp-f2" (usá las pantallas existentes
  como referencia de componentes)

## Decisiones cerradas que debés respetar (DESIGN.md §13.9.x)
1. Nav por estados: anónimo (CTA código escuela + iniciar sesión), invitado
   (Materiales/Proyectos), registrado con labels por rol (Aulas, no Salas).
2. Login unificado: email → si rol no-alumno → transición a página password.
   Nada de botón "Admin" visible en la nav anónima.
3. ALUMNOS SIN PASSWORD: magic link al email para emparejar su celular (PWA)
   una vez; después, login en PCs compartidas por QR inverso estilo WhatsApp Web:
   PC muestra QR efímero → alumno escanea con su celu → confirma ("¿Conectar con
   LAB-PC07?") → sesión temporal en la PC (expira sola).
4. Fallback sin celular: magic link desde email propio o creado en el momento,
   directamente en la PC.
5. Registro de alumnos CERRADO: solo docente/admin da de alta (import lista o
   invitación). No hay self-service abierto.
6. Notificaciones con badge en nav registrada.

## Tu trabajo: crear en el proyecto open-design ocicat-bella-mvp-f2

1. **login.html (REEMPLAZO)**: pantalla unificada — campo email único →
   detección: alumno → "te mandamos un link a tu email"; docente/director →
   transición a paso password. Link secundario "Ingresar con código" (escuela
   o sala). Sin registro self-service (texto informativo: "tu cuenta la crea
   tu docente").
2. **pair.html (NUEVA)**: pantalla QR inverso para PC compartida — QR grande
   centrado con countdown/regeneración, instrucción clara ("Escaneá con Ocicat
   en tu celular"), estado de espera → confirmación ("Conectado como Tizi").
3. **pwa-pair.html (NUEVA)**: pantalla del CELULAR — confirmación de conexión
   ("¿Conectar con LAB-PC07 ahora?"), lista de dispositivos propios, desvincular.
4. **nav-estados.html (NUEVA)**: página de referencia mostrando los 3 estados
   de navegación lado a lado (anónimo / invitado / registrado-alumno /
   registrado-docente) con labels correctos.

Usá los componentes existentes del proyecto (botones, cards, tipografía).
Pantallas estáticas HTML, livianas. Al terminar, listá los archivos creados.
