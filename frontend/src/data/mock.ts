// Datos mock del MVP — sin backend. Reemplazar por API real cuando exista.

export type Estado = 'running' | 'stopped' | 'historical';
export type Tipo = 'python' | 'web' | 'arduino';
export type Modo = 'job' | 'service';

export interface Sala {
  id: string;
  nombre: string;
  codigo: string;
  curso?: string;
  turno?: string;
  docente?: string;
  pendientes?: number;
}

export const salas: Sala[] = [
  { id: 'progra', nombre: 'Programación', curso: '5.º A', turno: 'Turno tarde', docente: 'Gabriel M.', codigo: 'PROG5A', pendientes: 3 },
  { id: 'base-datos', nombre: 'Base de Datos', curso: '5.º B', turno: 'Turno mañana', docente: 'Gabriel M.', codigo: 'BASE5B', pendientes: 0 },
  { id: 'redes', nombre: 'Redes y Comunicaciones', curso: '6.º B', turno: 'Turno completo', docente: 'Iara S.', codigo: 'REDE6B', pendientes: 1 },
  { id: 'robotica', nombre: 'Robótica 6°B', curso: '6.º B', turno: 'Turno mañana', docente: 'Luis P.', codigo: 'ROBO6B', pendientes: 0 },
];

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

export const getSandbox = (id: string) => sandboxes.find((s) => s.id === id);
export const salaDe = (id: string) => salas.find((s) => s.id === id);

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

// --- Materiales ---

export interface Material {
  id: string;
  salaId: string;
  etiqueta: 'PDF' | 'DOC' | 'VID' | 'PNG';
  nombre: string;
  tamano: string;
  fecha: string;
  autor: string;
}

export const materiales: Material[] = [
  { id: 'guia-listas-bucles', salaId: 'progra', etiqueta: 'PDF', nombre: 'Guía N°3: listas y bucles.pdf', tamano: '1,2 MB', fecha: '12/08', autor: 'Profe-Caro' },
  { id: 'video-contenedores', salaId: 'progra', etiqueta: 'VID', nombre: 'Video: ¿qué es un contenedor?.mp4', tamano: '48 MB', fecha: '10/08', autor: 'Profe-Caro' },
  { id: 'apuntes-diccionarios', salaId: 'progra', etiqueta: 'DOC', nombre: 'Apuntes: diccionarios en Python.doc', tamano: '240 kB', fecha: '8/08', autor: 'Alumno-PROG5A-03' },
  { id: 'tp-adivinanza', salaId: 'progra', etiqueta: 'DOC', nombre: 'Trabajo práctico: la adivinanza.doc', tamano: '96 kB', fecha: '5/08', autor: 'Profe-Caro' },
  { id: 'guia-condicionales', salaId: 'progra', etiqueta: 'PDF', nombre: 'Guía N°2: condicionales.pdf', tamano: '1,0 MB', fecha: '29/07', autor: 'Profe-Caro' },
  { id: 'esquema-semaforo', salaId: 'robotica', etiqueta: 'PNG', nombre: 'Robótica: esquema del semáforo.png', tamano: '310 kB', fecha: '25/07', autor: 'Profe-Luis' },
  { id: 'manual-arduino', salaId: 'robotica', etiqueta: 'PDF', nombre: 'Manual básico de Arduino.pdf', tamano: '3,4 MB', fecha: '22/07', autor: 'Profe-Luis' },
  { id: 'video-armado-semaforo', salaId: 'robotica', etiqueta: 'VID', nombre: 'Video: armado del semáforo.mp4', tamano: '62 MB', fecha: '20/07', autor: 'Profe-Luis' },
];

export const instaladores = [
  { nombre: 'Python', version: '3.12.4 · Win 64-bit · 26 MB' },
  { nombre: 'Visual Studio Code', version: '1.92 · Win 64-bit · 94 MB' },
  { nombre: 'Arduino IDE', version: '2.3.2 · Win 64-bit · 190 MB' },
  { nombre: 'Git', version: '2.46 · Win 64-bit · 58 MB' },
];

// --- Perfil y Sesiones del Alumno ---

export interface PerfilAlumno {
  nombre: string;
  email: string;
  iniciales: string;
  escuela: string;
}

export const perfilMock: PerfilAlumno = {
  nombre: 'Valentina Costa',
  email: 'vcosta@alumnos.epet.edu.ar',
  iniciales: 'VC',
  escuela: 'E.P.E.T. N° 1',
};

// --- Consignas y Entregas ---

export interface Consigna {
  id: string;
  salaId: string;
  titulo: string;
  runtime: 'Python' | 'Web' | 'C++' | 'Arduino';
  vence: string;
  vencida?: boolean;
  sinLimite?: boolean;
  descripcion: string;
  instrucciones?: string[];
  archivosAdjuntos?: { nombre: string; tamano: string }[];
  intentosMax: number;
  intentosUsados: number;
  estadoAlumno: 'pendiente' | 'entregado' | 'en_correccion' | 'aprobado';
  stats: {
    recibidas: number;
    total: number;
    ok: number;
    error: number;
    tardia: number;
    sinEntregar: number;
  };
}

export const consignas: Consigna[] = [
  {
    id: 'tp2',
    salaId: 'progra',
    titulo: 'TP2 — Manejo de archivos',
    runtime: 'Python',
    vence: 'Viernes 28 de agosto',
    descripcion: 'Subí tus archivos y elegí el entorno. Después podés probar en el sandbox (si querés) y entregar.',
    instrucciones: [
      'Crear un script `tarea.py` que procese un archivo de entrada y genere un reporte.',
      'Soportar manejo de errores para archivos inexistentes o mal formateados.',
      'Incluir una captura de pruebas locales `captura-tests.png` y el instructivo `README.md`.',
    ],
    archivosAdjuntos: [
      { nombre: 'enunciado-tp2.pdf', tamano: '140 KB' },
      { nombre: 'datos-ejemplo.csv', tamano: '18 KB' },
    ],
    intentosMax: 3,
    intentosUsados: 1,
    estadoAlumno: 'pendiente',
    stats: {
      recibidas: 18,
      total: 28,
      ok: 14,
      error: 3,
      tardia: 1,
      sinEntregar: 10,
    },
  },
  {
    id: 'tp1',
    salaId: 'progra',
    titulo: 'TP1 — Primeros pasos con Python',
    runtime: 'Python',
    vence: 'Viernes 28 de agosto',
    descripcion: 'Variables, tipos de datos básicos y operadores de comparación en Python.',
    intentosMax: 3,
    intentosUsados: 1,
    estadoAlumno: 'entregado',
    stats: {
      recibidas: 24,
      total: 28,
      ok: 20,
      error: 3,
      tardia: 1,
      sinEntregar: 4,
    },
  },
  {
    id: 'condicionales',
    salaId: 'progra',
    titulo: 'Práctica: condicionales',
    runtime: 'Python',
    vence: 'Miércoles 2 de septiembre',
    descripcion: 'Estructuras de control condicional `if`, `elif`, `else` y anidamiento.',
    intentosMax: 3,
    intentosUsados: 0,
    estadoAlumno: 'pendiente',
    stats: {
      recibidas: 19,
      total: 28,
      ok: 15,
      error: 3,
      tardia: 1,
      sinEntregar: 9,
    },
  },
  {
    id: 'calculadora',
    salaId: 'progra',
    titulo: 'Mini-proyecto: calculadora',
    runtime: 'Python',
    vence: 'Viernes 11 de septiembre',
    descripcion: 'Desarrollo de una calculadora interactiva por línea de comandos.',
    intentosMax: 3,
    intentosUsados: 0,
    estadoAlumno: 'pendiente',
    stats: {
      recibidas: 9,
      total: 28,
      ok: 7,
      error: 1,
      tardia: 1,
      sinEntregar: 19,
    },
  },
  {
    id: 'kata',
    salaId: 'progra',
    titulo: 'Kata de funciones',
    runtime: 'Python',
    vence: 'Sin fecha límite · práctica continua',
    sinLimite: true,
    descripcion: 'Ejercicios de práctica continua para reforzar definición y paso de parámetros en funciones.',
    intentosMax: 99,
    intentosUsados: 2,
    estadoAlumno: 'aprobado',
    stats: {
      recibidas: 14,
      total: 28,
      ok: 11,
      error: 2,
      tardia: 1,
      sinEntregar: 14,
    },
  },
  {
    id: 'repaso',
    salaId: 'progra',
    titulo: 'Repaso: listas y bucles',
    runtime: 'Python',
    vence: 'Venció el lunes 3 de agosto',
    vencida: true,
    descripcion: 'Repaso integrador de listas, tuplas, diccionarios y bucles `for` / `while`.',
    intentosMax: 3,
    intentosUsados: 1,
    estadoAlumno: 'aprobado',
    stats: {
      recibidas: 27,
      total: 28,
      ok: 21,
      error: 4,
      tardia: 2,
      sinEntregar: 1,
    },
  },
];

export const getConsigna = (id: string) => consignas.find((c) => c.id === id);

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

// --- Aulas Docente (B5) ---

export interface AulaDocente {
  id: string;
  nombre: string;
  codigo: string;
  curso: string;
  turno: string;
  especialidad: string;
  alumnosCount: number;
  consignasCount: number;
  sandboxesPrendidos: number;
}

export const aulasDocente: AulaDocente[] = [
  {
    id: 'prog5a',
    nombre: 'Programación',
    codigo: 'PROG5A',
    curso: '5.º A',
    turno: 'Turno tarde',
    especialidad: 'Técnica en Programación',
    alumnosCount: 28,
    consignasCount: 12,
    sandboxesPrendidos: 9,
  },
  {
    id: 'base5b',
    nombre: 'Base de Datos',
    codigo: 'BASE5B',
    curso: '5.º B',
    turno: 'Turno mañana',
    especialidad: 'Técnica en Programación',
    alumnosCount: 24,
    consignasCount: 8,
    sandboxesPrendidos: 4,
  },
  {
    id: 'redes6b',
    nombre: 'Redes y Comunicaciones',
    codigo: 'REDE6B',
    curso: '6.º B',
    turno: 'Turno completo',
    especialidad: 'Técnica en Computación',
    alumnosCount: 18,
    consignasCount: 5,
    sandboxesPrendidos: 2,
  },
  {
    id: 'robo6b',
    nombre: 'Robótica 6°B',
    codigo: 'ROBO6B',
    curso: '6.º B',
    turno: 'Turno mañana',
    especialidad: 'Técnica Electrónica',
    alumnosCount: 16,
    consignasCount: 4,
    sandboxesPrendidos: 0,
  },
];

// --- Alumnos de un Aula (B7) ---

export interface AlumnoItem {
  id: string;
  nombre: string;
  email: string;
  alias?: string;
  ultimaActividad: string;
  actividadReciente: boolean;
  entregasCount: number;
  vetado: boolean;
  estado: 'activo' | 'invitado_pendiente' | 'vetado';
}

export const alumnosMock: AlumnoItem[] = [
  {
    id: 'al-1',
    nombre: 'Valentina Costa',
    email: 'vcosta@alumnos.epet.edu.ar',
    alias: 'vale.c',
    ultimaActividad: 'Hoy 10:42',
    actividadReciente: true,
    entregasCount: 5,
    vetado: false,
    estado: 'activo',
  },
  {
    id: 'al-2',
    nombre: 'Mateo González',
    email: 'mgonzalez@alumnos.epet.edu.ar',
    alias: 'maty_gza',
    ultimaActividad: 'Hoy 08:15',
    actividadReciente: true,
    entregasCount: 4,
    vetado: false,
    estado: 'activo',
  },
  {
    id: 'al-3',
    nombre: 'Camila Ojeda',
    email: 'cojeda@alumnos.epet.edu.ar',
    alias: undefined,
    ultimaActividad: 'Ayer 17:30',
    actividadReciente: true,
    entregasCount: 5,
    vetado: false,
    estado: 'activo',
  },
  {
    id: 'al-4',
    nombre: 'Santiago Ríos',
    email: 'srios@alumnos.epet.edu.ar',
    alias: 'santii',
    ultimaActividad: 'Mié 19 · 21:04',
    actividadReciente: true,
    entregasCount: 3,
    vetado: false,
    estado: 'activo',
  },
  {
    id: 'al-5',
    nombre: 'Lucía Benítez',
    email: 'lbenitez@alumnos.epet.edu.ar',
    alias: undefined,
    ultimaActividad: 'Lun 17 · 19:22',
    actividadReciente: true,
    entregasCount: 4,
    vetado: false,
    estado: 'activo',
  },
  {
    id: 'al-6',
    nombre: 'Tomás Aguirre',
    email: 'taguirre@alumnos.epet.edu.ar',
    alias: 'tomi_dev',
    ultimaActividad: 'Hace 6 días',
    actividadReciente: false,
    entregasCount: 2,
    vetado: false,
    estado: 'activo',
  },
  {
    id: 'al-7',
    nombre: 'Micaela Sosa',
    email: 'msosa@alumnos.epet.edu.ar',
    alias: 'mica.sosa',
    ultimaActividad: 'Hace 2 semanas',
    actividadReciente: false,
    entregasCount: 1,
    vetado: false,
    estado: 'activo',
  },
  {
    id: 'al-8',
    nombre: 'Bruno Cabral',
    email: 'bcabral@alumnos.epet.edu.ar',
    alias: 'brunoc',
    ultimaActividad: 'Hace 3 semanas',
    actividadReciente: false,
    entregasCount: 1,
    vetado: true,
    estado: 'vetado',
  },
];

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

