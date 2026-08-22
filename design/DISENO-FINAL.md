# Ocicat Bella — Guía de diseño final (fusión c8e8 + c84c)

Fuente de verdad visual para la implementación. Combinar:

- **Estructura/UX**: `design/c8e8/` (Prototipo Web Escolar) — navegación, layout de páginas, jerarquía de componentes, app.js (interacciones mock).
- **Identidad visual**: `design/c84c/ocicat-editorial.css` — tokens, colores, tipografías.

## Tokens (de c84c)

```css
/* Colores */
--bg:#FAF7F2; --paper:#F5F1E9; --surface:#FFFEFC;
--ink:#1A1D23; --muted:#6B6E72; --muted2:#8A8F98;
--border:#E8E0D1; --border2:#DDD6C5;
--accent:#1B4332; --accent-2:#234E3B; --accent-hover:#143626;
--accent-light:#E6F0E9; --accent-mid:#B7D1C2;
--amber:#8A6D1B; --amber-bg:#FFF7D6; --amber-bd:#F2E3A3;
--radius:16px; --radius-sm:12px; --max:1180px; --header-h:64px;

/* Tipografías */
--font-display:'Instrument Serif', serif;      /* títulos, brand */
--font-body:'DM Sans', system-ui, sans-serif;  /* cuerpo, botones */
--font-mono:'JetBrains Mono', monospace;       /* código, logs, códigos de sala */
```

## Reglas de fusión

1. **Páginas**: usar la estructura HTML de c8e8 (index, materiales, unirse, sandboxes, sandbox-detalle).
2. **Estilos**: reemplazar el CSS de c8e8 por los tokens/clases de c84c (`ocicat-editorial.css`). Si una clase de estructura de c8e8 no existe en c84c, escribirla con los tokens de c84c.
3. **Logs/terminal**: usar `.mini-log` de c84c (fondo `#111827`, texto `#D1E5FF`, JetBrains Mono).
4. **Código de sala**: siempre JetBrains Mono, uppercase, letter-spacing.
5. **Estados**: running → verde accent; stopped → muted; historical → amber.
6. **Textos UI**: español rioplatense, tono simple y directo.

## Implementación

- Stack: Astro + Tailwind (tokens como CSS custom properties, sin daisyUI — el design system propio reemplaza a daisyUI).
- Directorio: `frontend/` (scaffold Astro mínimo, páginas estáticas + datos mock).
- Mockups de referencia: `design/c8e8/*.html` (estructura) + `design/c84c/ocicat-editorial.css` (estilo).
- Backend: NO en esta fase (datos mock en un `src/data/mock.ts`).
