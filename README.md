# Ocicat Bella

Plataforma FOSS para centralizar materiales educativos y ejecutar proyectos de alumnos en contenedores aislados.

## Alcance MVP
- Centralización de materiales (libros, docs, videos).
- Ejecución aislada de proyectos (Docker) para eliminar problemas de "en mi máquina funciona".
- Protocolo de federación simple para compartir proyectos entre instituciones.

## Stack
- **Backend:** FastAPI + SQLite (MVP simple, ligero, eficiente).
- **Ejecución:** Docker (contenedores efímeros).

## Cómo levantarlo
```bash
docker-compose up --build
```
Acceso: `http://localhost:8000`
