// API client: one fetch wrapper per platform service. The wire shapes come
// from the tygo-generated types (src/types/*) — the Go backend owns them.

const MERCHANTS =
  import.meta.env.VITE_MERCHANTS_URL ?? "https://merchants-bjm36sbwlq-uc.a.run.app";
const CATALOG =
  import.meta.env.VITE_CATALOG_URL ?? "https://catalog-bjm36sbwlq-uc.a.run.app";
const INVENTORY =
  import.meta.env.VITE_INVENTORY_URL ?? "https://inventory-bjm36sbwlq-uc.a.run.app";
const ORDERS =
  import.meta.env.VITE_ORDERS_URL ?? "https://orders-bjm36sbwlq-uc.a.run.app";

const TOKEN_KEY = "cc_admin_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken() {
  localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(base: string, path: string, init?: RequestInit): Promise<T> {
  const headers: Record<string, string> = {
    ...(init?.body ? { "Content-Type": "application/json" } : {}),
    ...(getToken() ? { Authorization: `Bearer ${getToken()}` } : {}),
  };
  const res = await fetch(`${base}${path}`, { ...init, headers });

  if (res.status === 401 && !path.startsWith("/v1/auth/")) {
    clearToken();
    window.location.assign("/login");
    throw new ApiError(401, "UNAUTHORIZED", "session expired");
  }
  if (!res.ok) {
    const body = await res.json().catch(() => ({}) as { code?: string; message?: string });
    throw new ApiError(res.status, body.code ?? "UNKNOWN", body.message ?? res.statusText);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const merchants = {
  get: <T>(path: string) => request<T>(MERCHANTS, path),
  post: <T>(path: string, body: unknown) =>
    request<T>(MERCHANTS, path, { method: "POST", body: JSON.stringify(body) }),
  put: <T>(path: string, body: unknown) =>
    request<T>(MERCHANTS, path, { method: "PUT", body: JSON.stringify(body) }),
  del: (path: string) => request<void>(MERCHANTS, path, { method: "DELETE" }),
};

export const catalog = {
  get: <T>(path: string) => request<T>(CATALOG, path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(CATALOG, path, { method: "POST", body: body ? JSON.stringify(body) : undefined }),
};

export const inventory = {
  get: <T>(path: string) => request<T>(INVENTORY, path),
  post: <T>(path: string, body: unknown) =>
    request<T>(INVENTORY, path, { method: "POST", body: JSON.stringify(body) }),
};

export const orders = {
  get: <T>(path: string) => request<T>(ORDERS, path),
  post: <T>(path: string, body: unknown) =>
    request<T>(ORDERS, path, { method: "POST", body: JSON.stringify(body) }),
};
