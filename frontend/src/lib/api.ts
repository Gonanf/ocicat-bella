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

/** GET /me (§11): campos extra sobre User + impersonated_by si aplica (§9.5). */
export interface MeUser extends User {
  school_name?: string;
  devices_paired?: number;
  impersonated_by?: string;
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

/** GET /auth/me o /me (§11, §9.5). */
export async function me(): Promise<MeUser> {
  try {
    const data = await request<MeUser | { user: MeUser; impersonated_by?: string }>('/api/v1/auth/me');
    if (data && typeof data === 'object' && 'user' in data && data.user) {
      return { ...data.user, impersonated_by: data.impersonated_by };
    }
    return data as MeUser;
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      return request<MeUser>('/api/v1/me');
    }
    throw err;
  }
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

export interface School {
  name: string;
  global_code: { code: string; active: boolean };
  stats: {
    teachers: number;
    classrooms: number;
    students: number;
    public_materials: number;
  };
}

export type SchoolInfo = School;

export interface Teacher {
  id: string;
  name: string;
  email: string;
  status: 'active' | 'disabled' | 'invited';
  classrooms_count?: number;
  total_submissions?: number;
  created_at?: string;
}

export type TeacherCredential =
  | { type: 'password'; password: string }
  | { type: 'magic_link' };

export interface CreateTeacherPayload {
  name: string;
  email: string;
  credential?: TeacherCredential;
}

export interface ImportRow {
  line: number;
  name: string;
  email: string;
  status: 'ok' | 'duplicate' | 'invalid_email' | 'exists' | string;
  reason?: string;
}

export interface ImportResult {
  import_token?: string;
  rows?: ImportRow[];
  summary?: { ok: number; rejected: number };
  created?: number;
  omitted?: number;
  warnings?: string[];
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

export function getSchool(): Promise<School> {
  return request('/api/v1/school');
}

/** El código anterior muere al instante (§9.1). Solo director. */
export function regenerateGlobalCode(): Promise<{ code: string }> {
  return request('/api/v1/school/global-code/regenerate', { method: 'POST' });
}

export async function deactivateGlobalCode(): Promise<void> {
  await request('/api/v1/school/global-code', { method: 'DELETE' });
}

export const disableGlobalCode = deactivateGlobalCode;

/** POST /guest/sessions — pública; 201 abre sesión invitado read-only. */
export function guestSession(code: string): Promise<{ school_name: string }> {
  return request('/api/v1/guest/sessions', json({ code }));
}

/** GET /school/public → { name } para la vista pública de escuela (§10). */
export function schoolPublic(): Promise<{ name: string }> {
  return request('/api/v1/school/public');
}

// --- Docentes (§9.2, CU-15) — Director ---

export async function listTeachers(): Promise<Teacher[]> {
  const data = await request<Teacher[] | { teachers: Teacher[] }>('/api/v1/school/teachers');
  return Array.isArray(data) ? data : data.teachers;
}

export async function createTeacher(payload: CreateTeacherPayload): Promise<Teacher> {
  const data = await request<Teacher | { teacher: Teacher }>('/api/v1/school/teachers', json(payload));
  return 'teacher' in data && data.teacher ? data.teacher : (data as Teacher);
}

export async function resetTeacherCredential(
  id: string,
  credential: TeacherCredential,
): Promise<Teacher> {
  const data = await request<Teacher | { teacher: Teacher }>(
    `/api/v1/school/teachers/${encodeURIComponent(id)}/reset-credential`,
    json({ credential }),
  );
  return 'teacher' in data && data.teacher ? data.teacher : (data as Teacher);
}

export async function disableTeacher(id: string): Promise<Teacher> {
  const data = await request<Teacher | { teacher: Teacher }>(
    `/api/v1/school/teachers/${encodeURIComponent(id)}/disable`,
    { method: 'POST' },
  );
  return 'teacher' in data && data.teacher ? data.teacher : (data as Teacher);
}

export async function enableTeacher(id: string): Promise<Teacher> {
  const data = await request<Teacher | { teacher: Teacher }>(
    `/api/v1/school/teachers/${encodeURIComponent(id)}/enable`,
    { method: 'POST' },
  );
  return 'teacher' in data && data.teacher ? data.teacher : (data as Teacher);
}

// --- Alta de alumnos y Magic Link (§9.3, CU-18) ---

export function importStudentsCsv(
  classroomId: string,
  file: File,
  dryRun = false,
  importToken?: string,
): Promise<ImportResult> {
  const fd = new FormData();
  fd.set('csv', file);
  fd.set('dry_run', String(dryRun));
  if (importToken) fd.set('import_token', importToken);
  return request(`/api/v1/classrooms/${encodeURIComponent(classroomId)}/students/import`, {
    method: 'POST',
    body: fd,
  });
}

export function inviteStudent(
  classroomId: string,
  payload: { name: string; email: string; send_invite?: boolean },
): Promise<{ student: { id?: string; name: string; email: string }; first_magic_link_sent: boolean }> {
  return request(`/api/v1/classrooms/${encodeURIComponent(classroomId)}/students/invite`, json(payload));
}

/** Reenviar magic link: 202 genérico anti-enumeración (§9.3). */
export function resendMagicLink(userId: string): Promise<{ sent: boolean; message: string }> {
  return request(`/api/v1/students/${encodeURIComponent(userId)}/resend-magic-link`, { method: 'POST' });
}

// --- Impersonación administrativa (§9.5) — Director ---

/** POST /admin/impersonations — 201 + sesión de impersonación (reason obligatorio). */
export function impersonate(
  userId: string,
  reason: string,
): Promise<{ user: User; impersonated_by: string }> {
  return request('/api/v1/admin/impersonations', json({ user_id: userId, reason }));
}

/** DELETE /admin/impersonations/current — 204 vuelve a sesión director. */
export async function stopImpersonation(): Promise<void> {
  await request('/api/v1/admin/impersonations/current', { method: 'DELETE' });
}

// --- Setup Wizard (§1, CU-12) ---

export function getSetupStatus(): Promise<{ configured: boolean }> {
  return request('/api/v1/setup/status');
}

export function setupSchool(name: string): Promise<{ school_id: string; name: string }> {
  return request('/api/v1/setup/school', json({ name }));
}

export function setupAdmin(payload: {
  name: string;
  email: string;
  password: string;
}): Promise<{ user: User; school: { global_code: string } }> {
  return request('/api/v1/setup/admin', json(payload));
}

// --- Consignas (§4) y Entregas (§5) — Fase 4 ---

/** Catálogo de runtimes (§10.2). Labels docentes en UI, no en API. */
export type Runtime = 'python' | 'web' | 'cpp' | 'arduino';

/** unlimited | limited{max} | one. */
export type AttemptsConfig = { mode: 'unlimited' } | { mode: 'limited'; max: number } | { mode: 'one' };

export type LatePolicy = 'allowed' | 'closed';

export interface AssignmentAttachment {
  attachment_id: string;
  filename: string;
  size_bytes: number;
}

export interface AssignmentStats {
  total_students: number;
  delivered: number;
  tested_ok: number;
  test_errors: number;
  late: number;
  missing: number;
}

export interface Assignment {
  id: string;
  classroom_id?: string;
  title: string;
  instructions?: string;
  runtime: Runtime;
  /** ISO; ausente = práctica continua. */
  due_at?: string;
  attempts: AttemptsConfig;
  late_policy: LatePolicy;
  attachments?: AssignmentAttachment[];
  /** Métricas cuando el listado las incluye para docente/director (§4). */
  stats?: AssignmentStats;
  created_at?: string;
}

/** Payload de creación/edición (§4): los adjuntos van por id, ya subidos a /assignments/attachments. */
export interface AssignmentDraft {
  title: string;
  instructions?: string;
  attachment_ids?: string[];
  runtime: Runtime;
  due_at?: string;
  attempts: AttemptsConfig;
  late_policy: LatePolicy;
}

export function createAssignment(classroomId: string, payload: AssignmentDraft): Promise<Assignment> {
  return request(`/api/v1/classrooms/${encodeURIComponent(classroomId)}/assignments`, json(payload));
}

export async function listAssignments(classroomId: string): Promise<Assignment[]> {
  const data = await request<Assignment[] | { assignments: Assignment[] }>(
    `/api/v1/classrooms/${encodeURIComponent(classroomId)}/assignments`,
  );
  return Array.isArray(data) ? data : data.assignments;
}

export function getAssignment(id: string): Promise<Assignment> {
  return request(`/api/v1/assignments/${encodeURIComponent(id)}`);
}

export function patchAssignment(id: string, patch: Partial<AssignmentDraft>): Promise<Assignment> {
  return request(`/api/v1/assignments/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(patch) });
}

/** Soft-delete: entregas conservadas. 204. */
export async function deleteAssignment(id: string): Promise<void> {
  await request(`/api/v1/assignments/${encodeURIComponent(id)}`, { method: 'DELETE' });
}

export function assignmentStats(id: string): Promise<AssignmentStats> {
  return request(`/api/v1/assignments/${encodeURIComponent(id)}/stats`);
}

/** POST /assignments/attachments — multipart, máx 10 MB/archivo. El id va en attachment_ids del create. */
export function uploadAttachment(file: File): Promise<AssignmentAttachment> {
  const fd = new FormData();
  fd.set('file', file);
  return request('/api/v1/assignments/attachments', { method: 'POST', body: fd });
}

// --- Entregas (§5): submission = UN intento. draft → delivered (+ flags derivados).

export interface SubmissionFile {
  id: string;
  name: string;
  size: number;
}

export type TestResult = { run_id: string; exit_code: number };

export interface Submission {
  id: string;
  assignment_id?: string;
  attempt_number: number;
  state: 'draft' | 'delivered';
  late?: boolean;
  tested_ok?: boolean;
  test_error?: boolean;
  delivered_at?: string;
  attempts_remaining?: number;
  last_test_result?: TestResult;
  files?: SubmissionFile[];
  student_name?: string;
}

/** POST /assignments/{id}/submissions/files — multipart multi; crea o reusa la draft del intento en curso. */
export function uploadSubmissionFiles(assignmentId: string, files: File[]): Promise<Submission> {
  const fd = new FormData();
  for (const f of files) fd.append('files', f);
  return request(`/api/v1/assignments/${encodeURIComponent(assignmentId)}/submissions/files`, {
    method: 'POST',
    body: fd,
  });
}

/**
 * POST /submissions/{id}/deliver — checkpoint explícito.
 * confirm_attempt = intento que el alumno vio en pantalla ("¿Entregar? (intento N de M)").
 * 409: attempts_exhausted | deadline_passed | attempt_conflict.
 */
export function deliverSubmission(submissionId: string, confirmAttempt: number): Promise<Submission> {
  return request(`/api/v1/submissions/${encodeURIComponent(submissionId)}/deliver`, json({ confirm_attempt: confirmAttempt }));
}

/** Historial de intentos propios (#N, tardía, etc.). */
export async function mySubmissions(assignmentId: string): Promise<Submission[]> {
  const data = await request<Submission[] | { submissions: Submission[] }>(
    `/api/v1/assignments/${encodeURIComponent(assignmentId)}/submissions/me`,
  );
  return Array.isArray(data) ? data : data.submissions;
}

export function getSubmission(id: string): Promise<Submission> {
  return request(`/api/v1/submissions/${encodeURIComponent(id)}`);
}

/** Vista corrección docente: filter=late|missing|error; sin filter = todas (§5.4). */
export async function listSubmissionsFiltered(
  assignmentId: string,
  filter?: 'late' | 'missing' | 'error',
): Promise<Submission[]> {
  const data = await request<Submission[] | { submissions: Submission[] }>(
    `/api/v1/assignments/${encodeURIComponent(assignmentId)}/submissions${qs({ filter })}`,
  );
  return Array.isArray(data) ? data : data.submissions;
}

// --- Sandboxes y Runs (§7), Templates (§10.2) y Settings (G4) — Fase 5 ---

export type SandboxMode = 'job' | 'service';
export type SandboxPurpose = 'standalone' | 'submission_test';
export type SandboxRetention = 'historical' | 'ephemeral';
export type RunStatus =
  | 'queued'
  | 'downloading_image'
  | 'starting'
  | 'ready'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'cleaned';

export interface Template {
  id: string;
  label: string;
  mode_default: SandboxMode;
  runtimes: Runtime[];
  packages_allowlist?: string[];
  start_command?: string;
}

export interface SandboxSettings {
  classroom_id?: string;
  allowed_templates: string[];
  custom_dockerfile_enabled?: boolean;
}

export interface CreateSandboxPayload {
  template_id: string;
  packages_extra?: string;
  mode?: SandboxMode;
  start_command?: string;
  purpose?: SandboxPurpose;
  submission_id?: string;
  retention?: SandboxRetention;
  visibility?: 'public' | 'school' | 'classroom';
}

export interface SandboxRunCreated {
  sandbox_id: string;
  run_id: string;
  status: RunStatus;
  queue_position?: number;
  budget_note?: string;
}

export interface RunHistoryEntry {
  n: number;
  outcome: 'success' | 'error' | 'timeout' | string;
}

export interface RunDetail {
  run_id: string;
  sandbox_id: string;
  status: RunStatus;
  exit_code: number | null;
  service_url: string | null;
  started_at: string | null;
  history?: RunHistoryEntry[];
}

export interface SandboxSummary {
  id: string;
  template_id: string;
  mode: SandboxMode;
  purpose: SandboxPurpose;
  retention: SandboxRetention;
  created_at: string;
  status?: RunStatus;
  author?: string;
}

export interface SandboxDetail extends SandboxSummary {
  start_command?: string;
  packages_extra?: string;
  visibility?: string;
  runs?: RunDetail[];
}

export function listTemplates(classroomId?: string): Promise<{ templates: Template[] }> {
  return request(`/api/v1/templates${qs({ classroom_id: classroomId })}`);
}

export async function getClassroomSettings(id: string): Promise<SandboxSettings> {
  const { templates } = await listTemplates(id);
  return {
    classroom_id: id,
    allowed_templates: templates.map((t) => t.id),
    custom_dockerfile_enabled: false,
  };
}

export function patchClassroomSettings(
  id: string,
  payload: { allowed_templates?: string[]; custom_dockerfile_enabled?: boolean },
): Promise<SandboxSettings> {
  return request(`/api/v1/classrooms/${encodeURIComponent(id)}/settings`, {
    method: 'PATCH',
    body: JSON.stringify(payload),
  });
}

export function createSandbox(
  payloadOrClassroomId: CreateSandboxPayload | string | undefined,
  maybePayload?: CreateSandboxPayload,
): Promise<SandboxRunCreated> {
  const payload: CreateSandboxPayload =
    typeof payloadOrClassroomId === 'object' && payloadOrClassroomId !== null
      ? payloadOrClassroomId
      : (maybePayload ?? { template_id: 'python/numpy' });
  return request('/api/v1/sandboxes', json(payload));
}

export function listSandboxes(): Promise<{ sandboxes: SandboxSummary[] }> {
  return request('/api/v1/sandboxes');
}

export function getSandbox(id: string): Promise<SandboxDetail> {
  return request(`/api/v1/sandboxes/${encodeURIComponent(id)}`);
}

export function instantiateSandbox(id: string): Promise<SandboxRunCreated> {
  return request(`/api/v1/sandboxes/${encodeURIComponent(id)}/instantiate`, { method: 'POST' });
}

export function getRun(runId: string): Promise<RunDetail> {
  return request(`/api/v1/runs/${encodeURIComponent(runId)}`);
}

export async function listRuns(sandboxId: string): Promise<RunDetail[]> {
  const sb = await getSandbox(sandboxId);
  return sb.runs ?? [];
}

export function stopRun(runId: string): Promise<{ status: string }> {
  return request(`/api/v1/runs/${encodeURIComponent(runId)}/stop`, { method: 'POST' });
}

export async function stopSandbox(idOrRunId: string): Promise<{ status: string }> {
  try {
    return await stopRun(idOrRunId);
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) {
      const sb = await getSandbox(idOrRunId);
      const runs = sb.runs;
      if (runs && runs.length > 0) {
        const lastRun = runs[runs.length - 1];
        return await stopRun(lastRun.run_id);
      }
    }
    throw err;
  }
}

export function runLogsUrl(runId: string): string {
  return `${BASE}/api/v1/runs/${encodeURIComponent(runId)}/logs`;
}

export function streamRunLogs(
  runId: string,
  handlers: {
    onLog?: (line: { stream: string; line: string }) => void;
    onStatus?: (status: { status: RunStatus }) => void;
    onExit?: (exit: { exit_code: number; status: RunStatus }) => void;
    onError?: (err: Event) => void;
  },
): EventSource {
  const url = runLogsUrl(runId);
  const es = new EventSource(url, { withCredentials: true });

  if (handlers.onLog) {
    es.addEventListener('log', (e) => {
      try {
        const data = JSON.parse(e.data);
        handlers.onLog?.(data);
      } catch {}
    });
  }

  if (handlers.onStatus) {
    es.addEventListener('status', (e) => {
      try {
        const data = JSON.parse(e.data);
        handlers.onStatus?.(data);
      } catch {}
    });
  }

  if (handlers.onExit) {
    es.addEventListener('exit', (e) => {
      try {
        const data = JSON.parse(e.data);
        handlers.onExit?.(data);
      } catch {}
    });
  }

  if (handlers.onError) {
    es.onerror = (e) => {
      handlers.onError?.(e);
    };
  }

  return es;
}

