# Análisis de Stack: Ocicat Bella

## 1. Tabla Comparativa de Lenguajes

| Lenguaje | Ecosistema (Web/Docker/SQLite) | Facilidad de Deploy | Mantenibilidad (No experto) | Seguridad/Aislamiento | Build/Size | Aptitud Runner |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| **Python** | Excelente | Runtime (venv) | Muy alta | Moderada | Lento / Grande | Media |
| **Rust** | Bueno | Binario único | Baja | Muy alta | Lento / Med | Alta |
| **TS** | Excelente | Runtime (Node) | Media | Moderada | Rápido / Grande | Media |
| **Go** | Excelente | Binario único | Alta | Alta | Muy rápido / Pequeño | Excelente |
| **Zig** | Emergente | Binario único | Muy baja | Alta | Instantáneo / Muy peq | Alta |

## 2. Arquitecturas Propuestas

*   **Python:** Backend (FastAPI), Runner (Docker SDK), UI (Jinja2 Templates). Mantener el scaffold actual, pero mejorar el aislamiento mediante `gVisor` o `nsjail`.
*   **Rust:** Backend (Axum), Runner (bollard), UI (Leptos/Templates). Seguridad máxima, pero costo de mantenimiento alto para personal no experto.
*   **TS:** Backend (Node/Express), Runner (Dockerode), UI (React/Next). Ecosistema familiar, pero requiere gestionar runtime (Node) y consumo de memoria elevado.
*   **Go:** Backend (Echo/Chi), Runner (Docker SDK), UI (Go Templates). El estándar de facto para infraestructura. Binario único facilita enormemente el deploy.
*   **Zig:** Backend (zap), Runner (syscalls directas), UI (Templates). Extremadamente eficiente, pero el ecosistema web/Docker no está lo suficientemente maduro para mantenimiento institucional.

## 3. Recomendación Justificada

**Recomendación: Go (Backend + Runner) + Go Templates (UI).**

*   **Justificación:**
    1.  **Binario único:** Elimina el "it works on my machine" y la dependencia de versiones de runtime (Node, Python). Es ideal para instituciones con poco presupuesto y personal no experto.
    2.  **Infraestructura Nativa:** La librería estándar de Go y el Docker SDK son extremadamente robustos y estables.
    3.  **Seguridad:** Go ofrece tipado estático y compilación eficiente, reduciendo errores comunes en producción sin la complejidad del borrow checker de Rust.
    4.  **UI:** Go Templates permite servir la UI directamente desde el binario. Evita el costo de un stack frontend pesado (JS/TS) sin sacrificar la simplicidad de deploy.

## 4. Decisiones Críticas del Usuario

1.  **¿Es aceptable renunciar a un SPA moderno (tipo React/Vue) a favor de un stack de Server-Side Rendering (Go Templates) para simplificar drásticamente el mantenimiento?**
2.  **¿Qué nivel de aislamiento es obligatorio para los proyectos de alumnos?** (Docker standard, gVisor, o Firecracker).
3.  **¿La federación de metadatos requiere una arquitectura peer-to-peer descentralizada (más compleja) o un relay centralizado (más simple)?**
