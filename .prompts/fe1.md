Estás implementando la FASE 1 de integración frontend de Ocicat Bella (repo actual, Astro + TypeScript en frontend/). El backend Go ya expone el contrato docs/api-v1.md — leé §0 (convenciones, errores uniformes {error:{code,message}}), §1 (setup wizard) y §2 (auth: login dos pasos, magic link, consume).

OBJETIVO: reemplazar los mocks de auth por llamadas reales al backend.

TAREAS:
1. Creá `frontend/src/lib/api.ts`: cliente API tipado — base URL configurable via `import.meta.env.PUBLIC_API_BASE` (default '' = mismo origen), fetch con `credentials: 'include'`, JSON, y manejo uniforme del error envelope `{error:{code,message}}` de §0 (throw ApiError con code+status+message; 429 incluye retry_after).
2. Tipos TS para User, SessionKind, respuestas de login (`{next}`), consume, /me.
3. Wire real de las páginas afectadas:
   - login.astro: POST /api/v1/auth/login/email → si next=password mostrar password y POST /auth/login/password; si next=magic_link mostrar pantalla magic link (POST /auth/magic-link context login_pc). Manejar 401 invalid_credentials, 403 account_disabled con copy del contrato.
   - index.astro o el flujo de consume: GET /api/v1/auth/consume?token=... manejando 410 token_expired/token_used, 400 invalid_token.
   - perfil.astro: GET /api/v1/me y DELETE /api/v1/sessions/current (logout).
4. Eliminá de src/data/mock.ts SOLO lo que queda reemplazado por estas llamadas (no rompas imports que otras páginas aún usan).
5. Mantené el estilo/diseño existente (Tailwind+daisyUI); sin cambios visuales grandes.

REGLAS:
- Commits convencionales (feat:/chore:). NO commitear .omo/, dist/, node_modules.
- Verificación obligatoria antes de terminar: `cd frontend && bun run build` debe pasar.
- Tests no hay infraestructura: validá con build y revisá tipos manualmente.

Terminá con resumen de archivos creados/modificados.
