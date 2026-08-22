# Plan Arquitectónico: Ocicat Bella

## Arquitectura
- **Backend:** FastAPI (Python) manejando la lógica, gestión de materiales y orquestación de Docker.
- **Base de Datos:** SQLite para el MVP (portabilidad).
- **Ejecución:** Docker (API de Docker para levantar contenedores).
- **Federación:** API REST para intercambio de metadatos (JSON).

## Roadmap
1. **Fase 1 (MVP):** Materiales, Registro de proyectos, Ejecución aislada básica.
2. **Fase 2 (Federación):** Protocolo básico de sincronización entre peers.
3. **Fase 3 (Refinamiento):** UI, Seguridad avanzada (seccomp, apparmor).

## Riesgos de Seguridad
- **Aislamiento:** Es el riesgo principal. Contenedores deben ejecutarse sin privilegios, con red restringida y filesystem efímero.
- **DoS:** Limitar recursos (CPU/RAM) por proyecto.
