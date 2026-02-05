import { getToken } from "./auth";

const API_BASE_URL = process.env.NEXT_PUBLIC_API_BASE_URL;

if (!API_BASE_URL) {
  throw new Error("Missing NEXT_PUBLIC_API_BASE_URL in .env.local");
}

export type ApiEnvelope<T> = {
  code: number;
  message: string;
  data: T;
};

export class ApiError extends Error {
  public code?: number;
  public status?: number;

  constructor(message: string, opts?: { code?: number; status?: number }) {
    super(message);
    this.name = "ApiError";
    this.code = opts?.code;
    this.status = opts?.status;
  }
}

async function parseJsonSafely(res: Response): Promise<any> {
  const text = await res.text();
  try {
    return text ? JSON.parse(text) : null;
  } catch {
    return null;
  }
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit & { auth?: boolean }
): Promise<T> {
  const url = `${API_BASE_URL}${path}`;
  const headers = new Headers(init?.headers);

  if (!headers.has("Content-Type") && init?.body) {
    headers.set("Content-Type", "application/json");
  }

  if (init?.auth) {
    const token = getToken();
    if (token) headers.set("Authorization", `Bearer ${token}`);
  }

  const res = await fetch(url, { ...init, headers });

  const json = (await parseJsonSafely(res)) as ApiEnvelope<T> | null;

  if (!res.ok) {
    throw new ApiError(json?.message ?? `HTTP ${res.status}`, { status: res.status });
  }
  if (!json) {
    throw new ApiError("Empty response from API", { status: res.status });
  }
  if (typeof json.code === "number" && json.code !== 0) {
    throw new ApiError(json.message || "API error", { code: json.code, status: res.status });
  }

  return json.data;
}