// Cliente API v1 de Ocicat Bella — contrato docs/api-v1.md
// Base configurable via PUBLIC_API_BASE ('' = mismo origen, típico en self-hosted).

const BASE: string = import.meta.env.PUBLIC_API_BASE ?? '';

/** Error uniforme del contrato §0: { error: { code, message } }. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  /** Segundos (solo 429 rate_limited). */
  readonly retryAfter?: number;

  constructor(status: number, code: string, message: string, retryAfter?: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.retryAfter = retryAfter;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    credentials: 'include',
    ...init,
    headers: {
      // multipart (FormData): el browser pone su propio Content-Type con boundary
      ...(init?.body && !(init.body instanceof FormData) ? { 'Content-Type': 'application/json' } : {}),
      ...init?.headers,
    },
  });

  if (!res.ok) {
    let code = 'unknown_error';
    let message = res.statusText || 'Ocurrió un error inesperado.';
    let retryAfter: number | undefined;

    try {
      const body = await res.json();
      if (body?.error) {
        code = body.error.code ?? code;
        message = body.error.message ?? message;
        if (typeof body.error.retry_after === 'number') retryAfter = body.error.retry_after;
      }
    } catch {
      // cuerpo no-JSON: quedan los defaults
    }
    if (retryAfter === undefined) {
      const header = res.headers.get('Retry-After');
      if (header && !Number.isNaN(Number(header))) retryAfter = Number(header);
    }

    throw new ApiError(res.status, code, message, retryAfter);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

function json(body: unknown): RequestInit {
  return { method: 'POST', body: JSON.stringify(body) };
}

// --- Tipos del contrato ---

export type Role = 'director' | 'docente' | 'alumno' | 'invitado';

/** §0.2: clases de sesión (consume devuelve pwa | pc_temporal). */
export type SessionKind = 'pwa' | 'staff' | 'pc_temporal';

export interface User {
  id: string;
  name: string;
  email: string;
  role: Role;
}

/** GET /me (§11): campos extra sobre User. */
export interface MeUser extends User {
  school_name?: string;
  devices_paired?: number;
}

export type MagicLinkContext = 'login_pc' | 'pairing_pwa';

// --- Endpoints de auth y perfil usados en Fase 1 ---

/** POST /auth/login/email — paso 1: detección de rol (§2.1). */
export function loginEmail(email: string): Promise<{ next: 'password' | 'magic_link' }> {
  return request('/api/v1/auth/login/email', json({ email }));
}

/** POST /auth/login/password — paso 2 docente/director (§2.1). */
export function loginPassword(email: string, password: string): Promise<{ user: User }> {
  return request('/api/v1/auth/login/password', json({ email, password }));
}

/** POST /auth/magic-link — pedir link de alumno (§2.2). 202 siempre si no hay rate limit. */
export function requestMagicLink(
  email: string,
  context: MagicLinkContext,
): Promise<{ sent: boolean; message: string }> {
  return request('/api/v1/auth/magic-link', json({ email, context }));
}

/** GET /auth/consume?token=... — validar magic link y abrir sesión (§2.2). */
export function consumeToken(token: string): Promise<{ user: User; session_kind: SessionKind }> {
  return request(`/api/v1/auth/consume?token=${encodeURIComponent(token)}`);
}

/** GET /me (§11). */
export function me(): Promise<MeUser> {
  return request('/api/v1/me');
}

/** DELETE /sessions/current — logout (§2.4). 204. */
export async function logout(): Promise<void> {
  await request<void>('/api/v1/sessions/current', { method: 'DELETE' });
}

// --- QR inverso (§2.3) y sesiones/dispositivos (§2.4) — Fase 2 ---

/** POST /auth/qr/start (201): sesión de emparejamiento creada por la PC. */
export interface PairingStart {
  pairing_id: string;
  /** URL del dominio oficial para el QR ([C3]). Single-use, TTL ≤90 s. */
  qr_url: string;
  expires_in: number;
  refresh_after: number;
}

export type PairingState = 'waiting' | 'scanned' | 'confirmed' | 'expired' | 'denied';

type PairingPoll =
  | { status: Exclude<PairingState, 'confirmed'> }
  | { status: 'confirmed'; user: { name: string; role: Role } };

/** POST /auth/qr/scan (200): identidad de la PC para confirmar ([C3]: label decorativo, IP+timestamp como prueba). */
export interface ScanInfo {
  pairing_id: string;
  device_label?: string;
  origin_ip?: string;
  requested_at: string;
}

export function qrStart(device_label?: string): Promise<PairingStart> {
  return request('/api/v1/auth/qr/start', device_label ? json({ device_label }) : { method: 'POST' });
}

export function qrStatus(pairing_id: string): Promise<PairingPoll> {
  return request(`/api/v1/auth/qr/${encodeURIComponent(pairing_id)}/status`);
}

export function qrScan(qr_token: string): Promise<ScanInfo> {
  return request('/api/v1/auth/qr/scan', json({ qr_token }));
}

/** 200 {} en éxito; claim atómico, concurrentes → 409 already_claimed. */
export async function qrConfirm(pairing_id: string): Promise<void> {
  await request(`/api/v1/auth/qr/${encodeURIComponent(pairing_id)}/confirm`, { method: 'POST' });
}

export async function qrDeny(pairing_id: string): Promise<void> {
  await request(`/api/v1/auth/qr/${encodeURIComponent(pairing_id)}/deny`, { method: 'POST' });
}

/** GET /sessions (§2.4): sesiones activas de la propia cuenta. */
export interface SessionInfo {
  id: string;
  kind: SessionKind;
  device_label?: string;
  created_at: string;
  last_seen_at: string;
}

export function listSessions(): Promise<{ sessions: SessionInfo[] }> {
  return request('/api/v1/sessions');
}

export async function deleteSession(id: string): Promise<void> {
  await request(`/api/v1/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export async function closeOthers(): Promise<void> {
  await request('/api/v1/sessions/close-others', { method: 'POST' });
}

// --- Aulas (§3) y Materiales (§6) — Fase 3 ---

export interface Classroom {
  id: string;
  name: string;
  course?: string;
  shift?: string;
  join_code?: string;
  /** Scope director: docente de cada aula. */
  teacher_name?: string;
  /** Stats scope docente. */
  students_count?: number;
  active_assignments?: number;
  running_sandboxes?: number;
  /** Scope alumno: consignas pendientes, o up_to_date=true. */
  pending_assignments?: number | null;
  up_to_date?: boolean;
}

export type MaterialType = 'pdf' | 'video' | 'image' | 'office' | 'other';
export type MaterialVisibility = 'public' | 'school' | 'classroom';

export interface Material {
  id: string;
  title: string;
  type: MaterialType;
  visibility: MaterialVisibility;
  subject?: string;
  preview_available: boolean;
  uploaded_by: string;
  created_at: string;
}

export interface StudentRow {
  user_id: string;
  name: string;
  email: string;
  status: 'invited' | 'active';
  submissions_count?: number;
}

export interface SchoolInfo {
  name: string;
  global_code: { code: string; active: boolean };
  stats: { teachers: number; classrooms: number; students: number; public_materials: number };
}

function qs(params: Record<string, string | undefined>): string {
  const usp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) if (v) usp.set(k, v);
  const s = usp.toString();
  return s ? `?${s}` : '';
}

export function createClassroom(body: { name: string; course?: string; shift?: string }): Promise<Classroom> {
  return request('/api/v1/classrooms', json(body));
}

/** GET /classrooms — scope por rol del contrato §3. */
export function listClassrooms(): Promise<{ classrooms: Classroom[] }> {
  return request('/api/v1/classrooms');
}

export function getClassroom(id: string): Promise<Classroom> {
  return request(`/api/v1/classrooms/${encodeURIComponent(id)}`);
}

export function patchClassroom(id: string, patch: Partial<Pick<Classroom, 'name' | 'course' | 'shift'>>): Promise<Classroom> {
  return request(`/api/v1/classrooms/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(patch) });
}

/** DELETE = archivar (entregas conservadas). 204. */
export async function deleteClassroom(id: string): Promise<void> {
  await request(`/api/v1/classrooms/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export function getJoinCode(id: string): Promise<{ code: string }> {
  return request(`/api/v1/classrooms/${encodeURIComponent(id)}/join_code`);
}

/** El código anterior deja de funcionar al instante (§3). */
export function rotateJoinCode(id: string): Promise<{ code: string }> {
  return request(`/api/v1/classrooms/${encodeURIComponent(id)}/join_code/rotate`, { method: 'POST' });
}

/** POST /classrooms/join — 404 invalid_code, 409 already_member. */
export function joinClassroom(code: string): Promise<{ classroom: Classroom }> {
  return request('/api/v1/classrooms/join', json({ code }));
}

/** Abandonar aula: entregas quedan archivadas. 204. */
export async function leaveClassroom(id: string): Promise<void> {
  await request(`/api/v1/classrooms/${encodeURIComponent(id)}/membership/me`, { method: 'DELETE' });
}

export async function listStudents(classroomId: string): Promise<StudentRow[]> {
  const data = await request<StudentRow[] | { students: StudentRow[] }>(
    `/api/v1/classrooms/${encodeURIComponent(classroomId)}/students`,
  );
  return Array.isArray(data) ? data : data.students;
}

/** Baja de alumno: entregas archivadas, cuenta intacta. 204. */
export async function removeStudent(classroomId: string, userId: string): Promise<void> {
  await request(
    `/api/v1/classrooms/${encodeURIComponent(classroomId)}/students/${encodeURIComponent(userId)}`,
    { method: 'DELETE' },
  );
}

/** GET /materials?scope=public|mine|classroom:{id}&subject=... — filtra según quien pregunta. */
export function listMaterials(opts?: { scope?: string; subject?: string }): Promise<{ materials: Material[] }> {
  return request(`/api/v1/materials${qs({ scope: opts?.scope, subject: opts?.subject })}`);
}

/** POST /classrooms/{id}/materials — multipart file/title/visibility/subject. */
export function uploadMaterial(
  classroomId: string,
  input: { file: File; title: string; visibility: MaterialVisibility; subject?: string },
): Promise<Material> {
  const fd = new FormData();
  fd.set('file', input.file);
  fd.set('title', input.title);
  fd.set('visibility', input.visibility);
  if (input.subject) fd.set('subject', input.subject);
  return request(`/api/v1/classrooms/${encodeURIComponent(classroomId)}/materials`, { method: 'POST', body: fd });
}

export function getMaterial(id: string): Promise<Material> {
  return request(`/api/v1/materials/${encodeURIComponent(id)}`);
}

/** Stream con Range; download fuerza Content-Disposition: attachment (§6). */
export function materialFileUrl(id: string, download = false): string {
  return `${BASE}/api/v1/materials/${encodeURIComponent(id)}/file${download ? '?download=1' : ''}`;
}

export function patchMaterial(id: string, patch: Partial<Pick<Material, 'title' | 'visibility' | 'subject'>>): Promise<Material> {
  return request(`/api/v1/materials/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(patch) });
}

export async function deleteMaterial(id: string): Promise<void> {
  await request(`/api/v1/materials/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

// --- Escuela e invitados (§9.1, §10) ---

export function getSchool(): Promise<SchoolInfo> {
  return request('/api/v1/school');
}

/** El código anterior muere al instante (§9.1). Solo director. */
export function regenerateGlobalCode(): Promise<{ code: string }> {
  return request('/api/v1/school/global-code/regenerate', { method: 'POST' });
}

export async function disableGlobalCode(): Promise<void> {
  await request('/api/v1/school/global-code', { method: 'DELETE' });
}

/** POST /guest/sessions — pública; 201 abre sesión invitado read-only. */
export function guestSession(code: string): Promise<{ school_name: string }> {
  return request('/api/v1/guest/sessions', json({ code }));
}

/** GET /school/public → { name } para la vista pública de escuela (§10). */
export function schoolPublic(): Promise<{ name: string }> {
  return request('/api/v1/school/public');
}
