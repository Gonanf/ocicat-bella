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
