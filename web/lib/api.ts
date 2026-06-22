// APAGE API client. Honors the cursor pagination, Idempotency-Key, and error
// envelope conventions (spec §"统一 API 约定", UI §12).

export type ApiError = {
  code: string;
  message: string;
  requestId?: string;
  retryable?: boolean;
};

export class ApiException extends Error {
  status: number;
  body: ApiError;
  constructor(status: number, body: ApiError) {
    super(body?.message || `HTTP ${status}`);
    this.status = status;
    this.body = body;
  }
}

export type List<T> = { items: T[]; nextCursor: string | null; hasMore: boolean };

let activeTenant: string | null = null;
export function setTenant(t: string | null) {
  activeTenant = t;
  if (typeof window !== "undefined") {
    if (t) localStorage.setItem("apage_tenant", t);
    else localStorage.removeItem("apage_tenant");
  }
}
export function getTenant(): string | null {
  if (activeTenant) return activeTenant;
  if (typeof window !== "undefined") activeTenant = localStorage.getItem("apage_tenant");
  return activeTenant;
}

type Opts = {
  method?: string;
  body?: unknown;
  tenant?: boolean; // attach X-Tenant-Id
  bearer?: string; // instance api key for data-plane calls
  idempotencyKey?: string;
};

// csrfToken reads the double-submit CSRF cookie set by the API (spec §25).
function csrfToken(): string | null {
  if (typeof document === "undefined") return null;
  const m = document.cookie.match(/(?:^|;\s*)apage_csrf=([^;]+)/);
  return m ? decodeURIComponent(m[1]) : null;
}

export async function api<T = unknown>(path: string, opts: Opts = {}): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (opts.tenant !== false) {
    const t = getTenant();
    if (t) headers["X-Tenant-Id"] = t;
  }
  if (opts.bearer) headers["Authorization"] = `Bearer ${opts.bearer}`;
  if (opts.idempotencyKey) headers["Idempotency-Key"] = opts.idempotencyKey;

  const method = opts.method || "GET";
  // Echo the CSRF cookie on state-changing, cookie-authenticated requests.
  if (method !== "GET" && method !== "HEAD" && !opts.bearer) {
    const c = csrfToken();
    if (c) headers["X-CSRF-Token"] = c;
  }

  const res = await fetch(`/api/v1${path}`, {
    method,
    headers,
    credentials: "include",
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    const err: ApiError = data?.error || { code: "UNKNOWN", message: `HTTP ${res.status}` };
    throw new ApiException(res.status, err);
  }
  return data as T;
}

export function idemKey(prefix = "ui"): string {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}
