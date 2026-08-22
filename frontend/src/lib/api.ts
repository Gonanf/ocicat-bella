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
      ...(init?.body ? { 'Content-Type': 'application/json' } : {}),
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
