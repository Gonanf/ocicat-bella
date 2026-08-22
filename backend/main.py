from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List
import docker

app = FastAPI()
client = docker.from_env()

class Project(BaseModel):
    name: str
    language: str # python, node, go

@app.get("/")
def read_root():
    return {"message": "Ocicat Bella Backend"}

@app.post("/projects/run")
def run_project(project: Project):
    # Esto es una implementación mínima para el scaffold
    try:
        # Lanzar contenedor aislado
        container = client.containers.run(
            f"ocicat-{project.language}-template",
            detach=True,
            mem_limit="128m",
            network_mode="none"
        )
        return {"status": "running", "id": container.short_id}
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
