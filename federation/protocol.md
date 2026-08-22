# Protocolo de Federación (Boceto)

El objetivo es compartir metadatos de proyectos entre servidores Ocicat.

## Formato de Intercambio (JSON)
```json
{
  "project_id": "uuid",
  "name": "Nombre del Proyecto",
  "language": "python",
  "author": "Alumno X"
}
```

## Endpoints
- `POST /federation/announce`: Recibe anuncio de nuevo proyecto.
- `GET /federation/sync`: Devuelve lista de proyectos locales.
