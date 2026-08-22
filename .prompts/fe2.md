Estás implementando la FASE 2 de integración frontend de Ocicat Bella (repo actual, Astro + TS en frontend/, ya existe src/lib/api.ts con ApiError y el contrato docs/api-v1.md §2.3 QR inverso + §2.4 sesiones/dispositivos).

OBJETIVO: wire real del QR inverso y gestión de dispositivos, reemplazando mocks.

TAREAS:
1. Extendé src/lib/api.ts con: qrStart(device_label?), qrStatus(pairing_id), qrScan(qr_token), qrConfirm(pairing_id), qrDeny(pairing_id), listSessions(), deleteSession(id), closeOthers(). Tipos PairingSession, SessionInfo.
2. pair.astro (la PC compartida):
   - Al cargar: POST /auth/qr/start → mostrar QR REAL (usá el qr_url devuelto; podés usar una librería de QR en cliente — preferí generar el QR con un módulo npm liviano tipo 'qrcode' agregado a dependencies, render en canvas/svg; NO dejar el SVG mock).
   - Poll GET /auth/qr/{id}/status cada ~3s (respetar expires_in/refresh_after: regenerar con nuevo qr/start al vencer). Estados waiting/scanned/confirmed/expired/denied con pantallas correspondientes ("revisá tu celular…" en scanned).
   - En confirmed: la respuesta trae Set-Cookie; redirigir según role (alumno → mis-salas).
   - Manejo de errores §0 con toasts existentes.
3. pwa-pair.astro o el flujo de escaneo del celular: POST /auth/qr/scan con el token escaneado → pantalla de confirmación mostrando device_label + origin_ip + requested_at ([C3]: label decorativo, IP+timestamp como prueba real), botones confirmar/deny → POST /auth/qr/{id}/confirm|deny.
   Si el escaneo se hace desde la cámara, dejá un input manual del token como fallback (no hay librería de cámara requerida).
4. perfil.astro: listar sesiones reales con GET /sessions (reemplazar placeholder visual), botón desvincular por sesión (DELETE /sessions/{id}) y "cerrar otras sesiones" (POST /sessions/close-others).
5. Limpiá de mock.ts solo lo que quede huérfano tras esto.

REGLAS:
- Mantené diseño Tailwind+daisyUI existente.
- Commits convencionales; NO commitear .omo/, dist/, node_modules.
- `cd frontend && bun run build` debe pasar antes de terminar.

Terminá con resumen de archivos creados/modificados.
