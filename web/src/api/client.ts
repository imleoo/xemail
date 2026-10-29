export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

let onUnauthorized: (() => void) | null = null

// 会话失效（非登录接口返回 401）时由应用层决定如何处理，避免 api 与 store 循环依赖。
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    ...init,
    headers: { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (res.status === 204) return undefined as T

  const data = await res.json().catch(() => null)
  if (!res.ok) {
    // /me 的 401 由 auth store 自己处理，避免处理器里再请求 /me 时递归
    if (res.status === 401 && !path.startsWith('/auth/') && path !== '/me') onUnauthorized?.()
    throw new ApiError(res.status, data?.error ?? res.statusText)
  }
  return data as T
}
