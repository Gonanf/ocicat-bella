// Datos mock residuales para el avatar/header en layout estático.
// Todas las entidades de negocio (aulas, materiales, alumnos, consignas, entregas, sandboxes) van contra la API real (src/lib/api.ts).

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



