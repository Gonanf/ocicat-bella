#!/usr/bin/env python3
"""od-run.py — corridas de diseño MVP Ocicat Bella en open-design.
Uso: python3 od-run.py [step]   (step 1..6, default: corre todos en secuencia)
"""
import json, sys, time, uuid, urllib.request

BASE         = "http://127.0.0.1:4098"
PROJECT      = "ocicat-bella-mvp-f2"
AGENT        = "opencode"
MODEL        = "opencode/x-preview-f-free"
POLL_SECS    = 25
RUN_TIMEOUT  = 480
QUOTA_WAIT   = 170
MAX_ATTEMPTS = 5

CTX = """Contexto del producto: Ocicat Bella, plataforma FOSS para escuelas.
Identidad visual YA DEFINIDA (usala exacta): fondo crema #FAF7F2 / superficie #FFFEFC,
tinta #1A1D23, verde bosque primario #1B4332, ámbar #8A6D1B; tipografias Instrument
Serif (titulos) + DM Sans (cuerpo) + JetBrains Mono (codigos/logs). Tema oscuro
coherente (#14161A fondo, #EDE8DD texto, verde #4E9B6F). Estilo editorial escolar,
limpio, espanol rioplatense en textos UI.
Modelos: Sala (codigo, docente, alumnos), Alumno (cuenta con nombre+email), Consigna,
Entrega, Sandbox (estado running/stopped/historical), Template de entorno.
Referencia de paginas ya disenadas: index (landing hero serif + CTAs), materiales
(sidebar materias + grid cards), unirse (card centrada input mono grande), sandboxes
(grid cards con badge estado), sandboxes/[id] (terminal mockup-code).
Genera HTML estatico autocontenido (un archivo por pantalla, mismo estilo que las
existentes). No uses frameworks JS.\n"""

STEPS = {
 "1": ("dashboard-docente", CTX + """Pantalla: PANEL DOCENTE — Mis salas (dashboard.html).
Header con logo Ocicat + nav (Salas, Materiales globales, Perfil) + toggle tema.
Hero corto "Buenas, Gabriel" + boton primario "+ Nueva sala".
Grid de cards de salas: nombre materia/curso, codigo mono grande (ej PROG5A),
N alumnos, N consignas activas, N sandboxes prendidos, acciones (Entrar, Codigo,
Config). Card "crear sala" punteada al final del grid.
Modal inline de nueva sala: nombre, curso, anio, generacion automatica de codigo."""),
 "2": ("consignas-docente", CTX + """Dos pantallas:
A) consignas.html — vista docente de consignas de una sala (Programacion 5A):
   lista con titulo, vencimiento, entregas recibidas/total, estados resumidos
   (OK/error/tardia), boton "+ Nueva consigna".
B) consigna-nueva.html — formulario de creacion: titulo, instrucciones (textarea),
   adjuntos opcionales (dropzone), runtime esperado (chips Python/Web/C++/Arduino),
   fecha limite (opcional), config de entrega como toggles/selects claros:
   intentos (ilimitados/N/uno solo), tardias permitidas ON/OFF.
   Nota visible: la config es modificable en cualquier momento."""),
 "3": ("alumnos-docente", CTX + """Dos pantallas:
A) alumnos.html — vista docente de alumnos de la sala: tabla con nombre real,
   email, alias opcional en sala, ultima actividad, entregas totales. Acciones por
   fila: ver perfil, vetar. Banner superior con el codigo de sala mono grande y
   boton "Rotar codigo" (con confirmacion inline explicando que mata accesos viejos).
B) sala-config.html — restricciones: seccion Templates (checkboxes de templates
   permitidos de un catalogo: bun/react, C++/sqlite, python/numpy, wordpress...),
   seccion Herramientas visibles (chips toggle con logo), toggle grande
   "Permitir Dockerfile propio" (default OFF, con advertencia)."""),
 "4": ("wizard-entrega", CTX + """Pantalla: WIZARD DE ENTREGA del alumno (entregar.html).
Stepper daisyUI-like de 4 pasos visible arriba (1 Consigna, 2 Archivos, 3 Probar,
4 Entregar), mostrando el paso 2-3 combinado: izquierda dropzone drag&drop con
archivos subidos listados (tarea.py, main.c...) y previews inline (icono/miniatura);
derecha terminal oscura (mockup-code estilo) mostrando la salida de la prueba
en sandbox efimero con boton "Probar mi codigo" y estado "prueba exitosa" verde.
Abajo barra de progreso: intentos usados 2/3, boton primario "Entregar".
Incluir version estatica del paso 4: card de confirmacion con resumen."""),
 "5": ("composer-templates", CTX + """Dos pantallas:
A) nuevo-sandbox.html — selector de template: grid de cards grandes con logo
   (Bun, React, C++, Python, WordPress, Arduino), nombre, descripcion corta,
   comando default en mono, badge modo (job/service). Card final "Armar el mio"
   que lleva al composer.
B) composer.html — composer visual: izquierda catalogo de herramientas como
   botones con logo (WordPress, C++, SQLite, Python, Bun, Node, nginx); derecha
   panel "Tu entorno" que se va llenando con las herramientas elegidas (chips),
   comando de inicio editable en mono, preview del Dockerfile generado (mono,
   oscuro) y advertencias de seguridad si corresponde. Boton "Guardar template"."""),
 "6": ("perfil-alumno", CTX + """Tres pantallas:
A) login.html — login/registro alumno unificado: card centrada con tabs
   "Entrar" / "Crear cuenta"; crear cuenta pide nombre, email, password.
   Debajo bloque separado "Ya tenes codigo de sala?" con input mono.
B) mis-salas.html — salas del alumno: cards con sala, rol, su alias en esa sala,
   consignas pendientes count; accion "Abandonar sala" con confirmacion inline
   explicando que sus entregas quedan archivadas.
C) perfil.html — datos minimos: nombre, email, cambiar password, sesiones
   activas (lista dispositivos con "cerrar otras sesiones"), apodos por sala."""),
}

def post_run(prompt):
    body = {"projectId": PROJECT, "clientRequestId": str(uuid.uuid4()),
            "message": prompt, "currentPrompt": prompt, "agentId": AGENT, "model": MODEL}
    req = urllib.request.Request(f"{BASE}/api/runs", data=json.dumps(body).encode(),
                                 headers={"Content-Type": "application/json"}, method="POST")
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)["runId"]

def poll(run_id):
    deadline = time.time() + RUN_TIMEOUT
    while time.time() < deadline:
        time.sleep(POLL_SECS)
        with urllib.request.urlopen(f"{BASE}/api/runs/{run_id}", timeout=30) as r:
            d = json.load(r)
        print("  STATUS", d.get("status"), flush=True)
        if d.get("status") in ("succeeded", "failed", "canceled"):
            return d
    return {"status": "timeout"}

def main():
    steps = sys.argv[1:] or list(STEPS)
    for s in steps:
        name, prompt = STEPS[s]
        print(f"=== STEP {s}: {name} ===", flush=True)
        ok = False
        for attempt in range(1, MAX_ATTEMPTS + 1):
            print(f"[attempt {attempt}/{MAX_ATTEMPTS}]", flush=True)
            run_id = post_run(prompt)
            print("  RUN", run_id, flush=True)
            result = poll(run_id)
            if result.get("status") == "succeeded":
                print(json.dumps(result.get("artifactPaths", result), indent=2)[:500], flush=True)
                ok = True
                break
            if result.get("failureCategory") == "rate_limit":
                print(f"  quota hit, cooldown {QUOTA_WAIT}s…", flush=True)
                time.sleep(QUOTA_WAIT)
            else:
                print("  failed:", json.dumps(result)[:300], flush=True)
                time.sleep(10)
        if not ok:
            print(f"STEP {s} ({name}) AGOTADO", flush=True)
            return 1
    print("TODOS LOS STEPS OK", flush=True)
    return 0

if __name__ == "__main__":
    sys.exit(main())
