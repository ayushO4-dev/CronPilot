// Thin fetch wrapper. Cookies are sent with every request (credentials:
// 'include') so the session cookie authenticates the API.

export interface ApiError {
  error: string
  status: number
  /** Set by the login endpoint when a valid password still needs a 2FA code. */
  totpRequired?: boolean
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
    },
    ...options,
  })

  if (res.status === 204) {
    return undefined as T
  }

  const text = await res.text()
  const data = text ? JSON.parse(text) : null

  if (!res.ok) {
    const err: ApiError = {
      error: (data && data.error) || res.statusText || 'request failed',
      status: res.status,
    }
    if (data && data.totpRequired) err.totpRequired = true
    throw err
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>(path),
  post: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) }),
  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', body: body === undefined ? undefined : JSON.stringify(body) }),
}

export function isApiError(e: unknown): e is ApiError {
  return typeof e === 'object' && e !== null && 'error' in e && 'status' in e
}
