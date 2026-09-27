import type { User } from "./types";

export const API = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
export const WS = process.env.NEXT_PUBLIC_WS_URL ?? API.replace(/^http/, "ws") + "/api/v1/realtime";

const TOKEN = "metronav.token";
const USER = "metronav.user";

export function session(): { token: string; user: User } | null {
  if (typeof window === "undefined") return null;
  try {
    const token = localStorage.getItem(TOKEN);
    const user = localStorage.getItem(USER);
    return token && user ? { token, user: JSON.parse(user) } : null;
  } catch {
    return null;
  }
}

export function saveSession(token: string, user: User) {
  localStorage.setItem(TOKEN, token);
  localStorage.setItem(USER, JSON.stringify(user));
  window.dispatchEvent(new Event("metronav-session"));
}

export function signOut() {
  localStorage.removeItem(TOKEN);
  localStorage.removeItem(USER);
  window.dispatchEvent(new Event("metronav-session"));
}

export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const s = session();
  const headers: Record<string, string> = { "Content-Type": "application/json", ...(init.headers as Record<string, string>) };
  if (s) headers.Authorization = `Bearer ${s.token}`;
  let res: Response;
  try {
    res = await fetch(API + path, { ...init, headers, cache: "no-store" });
  } catch {
    throw new ApiError(0, `Can't reach the MetroNav API at ${API}. Check that the core service is running.`);
  }
  if (res.status === 401 && s) signOut();
  if (!res.ok) {
    const body = await res.json().catch(() => ({}));
    throw new ApiError(res.status, body.error ?? `Request failed (${res.status})`);
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

export const isStaff = (role?: string) => role === "OPERATOR" || role === "ADMIN" || role === "ANALYST";
export const canOperate = (role?: string) => role === "OPERATOR" || role === "ADMIN";
