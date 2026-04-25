export function applyInterceptWarmup(
  credentials: Record<string, unknown>,
  enabled: boolean,
  mode: 'create' | 'edit'
): void {
  if (enabled) {
    credentials.intercept_warmup_requests = true
  } else if (mode === 'edit') {
    delete credentials.intercept_warmup_requests
  }
}

export const OPENAI_COMPAT_PROVIDER_NAME_EXTRA_KEY = 'openai_compat_provider_name'

export function parseOpenAICompatHeaders(
  input: string
): Record<string, string> {
  const trimmed = input.trim()
  if (!trimmed) {
    return {}
  }

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    throw new Error('invalid_openai_compat_headers')
  }

  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('invalid_openai_compat_headers')
  }

  const headers: Record<string, string> = {}
  for (const [key, value] of Object.entries(parsed as Record<string, unknown>)) {
    const headerKey = key.trim()
    if (!headerKey) {
      continue
    }
    if (typeof value !== 'string') {
      throw new Error('invalid_openai_compat_headers')
    }
    const headerValue = value.trim()
    if (!headerValue) {
      continue
    }
    headers[headerKey] = headerValue
  }

  return headers
}

export function formatOpenAICompatHeaders(
  raw: unknown
): string {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return ''
  }

  const headers: Record<string, string> = {}
  for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
    const headerKey = key.trim()
    if (!headerKey || typeof value !== 'string') {
      continue
    }
    const headerValue = value.trim()
    if (!headerValue) {
      continue
    }
    headers[headerKey] = headerValue
  }

  if (Object.keys(headers).length === 0) {
    return ''
  }

  return JSON.stringify(headers, null, 2)
}
