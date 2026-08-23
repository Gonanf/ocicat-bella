// Datos mock del MVP — solo fases aún sin backend (sandboxes FE5).
// Aulas/materiales/alumnos/consignas/entregas ya van contra la API real (src/lib/api.ts).

export type Estado = 'running' | 'stopped' | 'historical';
export type Tipo = 'python' | 'web' | 'arduino';
export type Modo = 'job' | 'service';

export interface LogLine {
  /** prefijo del prompt ('$', '>'); vacío para salida */
  p?: string;
  t: string;
}

export interface Ejecucion {
  n: number;
  resultado: string;
  exit: number;
  cuando: string;
}

export interface Sandbox {
  id: string;
  numero: number;
  titulo: string;
  tipo: Tipo;
  modo: Modo;
  estado: Estado;
  salaId: string;
  autor: string;
  lenguaje: string;
  url?: string;
  limites: string;
  ultimaEjecucion: string;
  ultimoExit: number;
  logs: LogLine[];
  historial: Ejecucion[];
}

export const CAPACIDAD = 4; // sandboxes simultáneos en el server de la escuela

export const sandboxes: Sandbox[] = [
  {
    id: 'adivinanza-numerica',
    numero: 42,
    titulo: 'Adivinanza numérica',
    tipo: 'python',
    modo: 'job',
    estado: 'running',
    salaId: 'progra',
    autor: 'Alumno-PROG5A-12',
    lenguaje: 'Python 3.11',
    limites: '128 MB · 120 s · sin red',
    ultimaEjecucion: 'hoy 14:32',
    ultimoExit: 0,
    logs: [
      { p: '$', t: 'python main.py' },
      { t: '[ok] contenedor sb-42 arriba (imagen python:3.11-slim)' },
      { t: 'Pienso un número entre 1 y 100. Adivinalo...' },
      { t: 'turno 1 -> 50 (es menor)' },
      { t: 'turno 2 -> 25 (es mayor)' },
      { t: 'turno 3 -> 37 (es mayor)' },
      { t: 'turno 4 -> 43 (¡correcto!)' },
      { t: '[exit 0] ejecución terminada en 3,2 s' },
    ],
    historial: [
      { n: 7, resultado: 'Exitosa', exit: 0, cuando: 'hoy 14:32' },
      { n: 6, resultado: 'Falló: SyntaxError en línea 12', exit: 1, cuando: 'ayer 15:10' },
      { n: 5, resultado: 'Timeout: se pasó de los 120 s', exit: 124, cuando: '12/08 10:05' },
    ],
  },
  {
    id: 'blog-del-curso',
    numero: 44,
    titulo: 'Blog del curso',
    tipo: 'web',
    modo: 'service',
    estado: 'running',
    salaId: 'progra',
    autor: 'Alumno-PROG5A-21',
    lenguaje: 'Node 20 + Vite',
    url: 'ocicat.local/s/44',
    limites: '256 MB · sin límite de tiempo · red interna',
    ultimaEjecucion: 'hoy 09:15',
    ultimoExit: 0,
    logs: [
      { p: '$', t: 'npm run dev -- --host 0.0.0.0' },
      { t: '[ok] contenedor sb-44 arriba (imagen node:20-alpine)' },
      { t: 'VITE v5.4.2 ready en 412 ms' },
      { t: '-> local:   http://localhost:5173/' },
      { t: '-> red:     expuesto como ocicat.local/s/44' },
      { t: 'GET /            200 · 12 ms' },
      { t: 'GET /posts       200 · 8 ms' },
      { t: 'GET /favicon.ico 200 · 3 ms' },
    ],
    historial: [
      { n: 4, resultado: 'Exitosa', exit: 0, cuando: 'hoy 09:15' },
      { n: 3, resultado: 'Exitosa', exit: 0, cuando: 'ayer 08:40' },
      { n: 2, resultado: 'Falló: puerto 5173 ocupado', exit: 1, cuando: '13/08 10:22' },
    ],
  },
  {
    id: 'mi-primer-html',
    numero: 51,
    titulo: 'Mi primer HTML',
    tipo: 'web',
    modo: 'job',
    estado: 'stopped',
    salaId: 'progra',
    autor: 'Valen (Alumno-PROG5A-07)',
    lenguaje: 'Python 3.11 · http.server',
    limites: '64 MB · 60 s · sin red',
    ultimaEjecucion: 'ayer 16:48',
    ultimoExit: 0,
    logs: [
      { p: '$', t: 'python -m http.server 8000' },
      { t: '[ok] contenedor sb-51 arriba (imagen python:3.11-slim)' },
      { t: 'Serving HTTP on 0.0.0.0 port 8000' },
      { t: 'GET /index.html 200 · 5 ms' },
      { t: 'GET /style.css  200 · 4 ms' },
      { t: '[stop] detenido por el alumno (SIGTERM)' },
      { t: '[exit 0] contenedor apagado limpio' },
    ],
    historial: [
      { n: 9, resultado: 'Detenido por el alumno', exit: 0, cuando: 'ayer 16:48' },
      { n: 8, resultado: 'Exitosa', exit: 0, cuando: 'ayer 16:41' },
      { n: 7, resultado: 'Falló: index.html no encontrado', exit: 1, cuando: 'ayer 16:35' },
    ],
  },
  {
    id: 'semaforo-leds',
    numero: 63,
    titulo: 'Semáforo con LEDs',
    tipo: 'arduino',
    modo: 'job',
    estado: 'historical',
    salaId: 'robotica',
    autor: 'curso Robótica 6°B',
    lenguaje: 'Arduino Uno (simulación)',
    limites: '128 MB · 120 s · puerto serie virtual',
    ultimaEjecucion: '18/08 11:02',
    ultimoExit: 124,
    logs: [
      { p: '$', t: 'arduino-cli compile --fqbn arduino:uno:semaforo' },
      { t: 'Sketch usa 3456 bytes (10%) de la memoria flash' },
      { t: 'Variables globales usan 210 bytes de RAM' },
      { p: '$', t: 'arduino-cli simulate --cycles 3' },
      { t: '[ok] subido al Arduino Uno (virtual)' },
      { t: 'ciclo 1: rojo 5s -> amarillo 2s -> verde 4s' },
      { t: 'ciclo 2: rojo 5s -> amarillo 2s -> verde 4s' },
      { t: 'ciclo 3: rojo 5s -> amarillo 2s ->' },
      { t: '[exit 124] timeout de simulación a los 120 s, contenedor limpiado' },
      { t: '[histórico] registro conservado, re-instanciable desde el panel docente' },
    ],
    historial: [
      { n: 3, resultado: 'Timeout: se pasó de los 120 s', exit: 124, cuando: '18/08 11:02' },
      { n: 2, resultado: 'Exitosa (3 ciclos)', exit: 0, cuando: '18/08 10:47' },
      { n: 1, resultado: 'Falló: compilación, pin 13 duplicado', exit: 1, cuando: '15/08 09:30' },
    ],
  },
];

// ponytail: salas mínimas para los mocks de sandboxes (FE5); cuando sandboxes van contra API, esto muere.
const salasSandbox = {
  progra: { id: 'progra', nombre: 'Programación', codigo: 'PROG5A' },
  robotica: { id: 'robotica', nombre: 'Robótica', codigo: 'ROBO6B' },
} as const;

export const getSandbox = (id: string) => sandboxes.find((s) => s.id === id);
export const salaDe = (id: string) => salasSandbox[id as keyof typeof salasSandbox];

export const estadoInfo: Record<Estado, { label: string; badge: string }> = {
  running: { label: 'corriendo', badge: 'badge-success badge-soft' },
  stopped: { label: 'detenido', badge: 'badge-ghost text-muted' },
  historical: { label: 'histórico', badge: 'badge-warning badge-soft' },
};

export const tipoLabel: Record<Tipo, string> = {
  python: 'Python',
  web: 'Web',
  arduino: 'Arduino',
};

// --- Perfil y Sesiones del Alumno ---

// --- Roles y Usuarios (B1, B2) ---

export type Rol = 'anonimo' | 'invitado' | 'alumno' | 'docente' | 'director';

export interface UsuarioMock {
  id: string;
  nombre: string;
  email: string;
  rol: Rol;
  iniciales: string;
  subtitulo: string;
  notificacionesCount: number;
}

export const usuariosMock: Record<Rol, UsuarioMock> = {
  anonimo: {
    id: 'u-anon',
    nombre: 'Visitante',
    email: '',
    rol: 'anonimo',
    iniciales: 'V',
    subtitulo: 'Sin cuenta',
    notificacionesCount: 0,
  },
  invitado: {
    id: 'u-guest',
    nombre: 'Invitado Escuela',
    email: '',
    rol: 'invitado',
    iniciales: 'IE',
    subtitulo: 'E.E.S.T. N° 1',
    notificacionesCount: 0,
  },
  alumno: {
    id: 'u-alumno',
    nombre: 'Tiziano Maidana',
    email: 'tiziano.maidana@alumnos.epet1.edu.ar',
    rol: 'alumno',
    iniciales: 'TM',
    subtitulo: '5° 1° Computación',
    notificacionesCount: 3,
  },
  docente: {
    id: 'u-docente',
    nombre: 'Prof. Roberto García',
    email: 'profe.garcia@epet1.edu.ar',
    rol: 'docente',
    iniciales: 'RG',
    subtitulo: '4 Aulas activas',
    notificacionesCount: 5,
  },
  director: {
    id: 'u-director',
    nombre: 'Dirección E.T. N°1',
    email: 'direccion@epet1.edu.ar',
    rol: 'director',
    iniciales: 'DIR',
    subtitulo: 'Administrador',
    notificacionesCount: 1,
  },
};

export const currentMockRole: Rol = 'docente';
export const currentMockUser = usuariosMock[currentMockRole];

// --- Configuración de Sala (B8) ---

export interface TemplateEntorno {
  id: string;
  nombre: string;
  descripcion: string;
  habilitado: boolean;
}

export const templatesEntornos: TemplateEntorno[] = [
  {
    id: 'python-numpy',
    nombre: 'python/numpy',
    descripcion: 'Python 3.12 con NumPy y Matplotlib precargados',
    habilitado: true,
  },
  {
    id: 'cpp-sqlite',
    nombre: 'c++/sqlite',
    descripcion: 'GCC 13 con SQLite3 para bases embebidas',
    habilitado: false,
  },
  {
    id: 'bun-react',
    nombre: 'bun/react',
    descripcion: 'Bun 1.1 con React + Vite listos para SPA',
    habilitado: false,
  },
  {
    id: 'node-express',
    nombre: 'node/express',
    descripcion: 'Node 22 con Express para APIs REST',
    habilitado: false,
  },
  {
    id: 'wordpress',
    nombre: 'wordpress',
    descripcion: 'PHP 8.3 con WordPress + MariaDB',
    habilitado: false,
  },
  {
    id: 'java-maven',
    nombre: 'java/maven',
    descripcion: 'OpenJDK 21 con Maven y JUnit',
    habilitado: false,
  },
];

export interface HerramientaVisible {
  id: string;
  nombre: string;
  icono: string;
  activa: boolean;
}

export const herramientasVisibles: HerramientaVisible[] = [
  { id: 'vscode', nombre: 'VS Code', icono: '</>', activa: true },
  { id: 'terminal', nombre: 'Terminal', icono: '>_', activa: true },
  { id: 'git', nombre: 'Git', icono: 'G', activa: true },
  { id: 'navegador', nombre: 'Navegador', icono: 'N', activa: false },
  { id: 'docs', nombre: 'Docs', icono: 'D', activa: false },
  { id: 'asistente-ia', nombre: 'Asistente IA', icono: 'IA', activa: false },
];

