export interface ApiErrorPayload {
  code: string;
  message: string;
  request_id: string;
  details: Record<string, string>;
}

interface ErrorEnvelope {
  error: ApiErrorPayload;
}

interface DataEnvelope<T> {
  data: T;
  meta?: Record<string, unknown>;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string;
  readonly details: Record<string, string>;

  constructor(status: number, payload: ApiErrorPayload) {
    super(payload.message);
    this.name = 'ApiError';
    this.status = status;
    this.code = payload.code;
    this.requestId = payload.request_id;
    this.details = payload.details;
  }
}

// OfflineError replaces a raw fetch-level network failure (a TypeError
// with an unhelpful browser message like "Failed to fetch") with a plain
// statement of what actually happened: the device has no connection, and
// the action needs one. Every genuinely offline-capable flow in this app
// (Sell.tsx's IndexedDB queue, salesSync.ts) already avoids calling
// apiRequest until it's ready to sync -- this is for the many other
// activities (tabs, invitations, stock, signup, onboarding, platform
// admin) that have no offline path and simply cannot proceed without one.
//
// Deliberately NOT a proactive navigator.onLine check before the fetch --
// per useConnectivity.ts's own caveat, that flag is a hint, not proof (it
// can read true with no real connectivity, or lag behind reality); the one
// thing that reliably indicates a request couldn't go anywhere is fetch
// itself throwing, so that's what this catches.
export class OfflineError extends Error {
  constructor() {
    super("You're offline. This needs an internet connection to work.");
    this.name = 'OfflineError';
  }
}

export async function apiRequest<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set('Accept', 'application/json');
  if (init.body != null && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }

  let response: Response;
  try {
    response = await fetch(path, {
      ...init,
      credentials: 'same-origin',
      headers,
    });
  } catch {
    // fetch throwing outright (not an HTTP error status) is, for this
    // same-origin API, almost always a connectivity problem -- a dropped
    // connection mid-request, a DNS failure, or navigator.onLine not
    // having updated yet when the request started. Treat it the same as
    // the explicit offline check above rather than surfacing the raw,
    // cryptic TypeError.
    throw new OfflineError();
  }

  const payload = await parsePayload(response);
  if (!response.ok) {
    const error = (payload as Partial<ErrorEnvelope> | null)?.error;
    if (isApiErrorPayload(error)) {
      throw new ApiError(response.status, error);
    }
    throw new ApiError(response.status, {
      code: 'UNEXPECTED_RESPONSE',
      message: 'The server returned an unexpected response.',
      request_id: response.headers.get('X-Request-ID') ?? '',
      details: {},
    });
  }

  if (response.status === 204) {
    return undefined as T;
  }
  const data = (payload as Partial<DataEnvelope<T>> | null)?.data;
  if (data === undefined) {
    throw new ApiError(response.status, {
      code: 'UNEXPECTED_RESPONSE',
      message: 'The server response did not include data.',
      request_id: response.headers.get('X-Request-ID') ?? '',
      details: {},
    });
  }
  return data;
}

async function parsePayload(response: Response): Promise<unknown> {
  if (response.status === 204) {
    return null;
  }
  const contentType = response.headers.get('Content-Type') ?? '';
  if (!contentType.includes('application/json')) {
    return null;
  }
  try {
    return await response.json();
  } catch {
    return null;
  }
}

function isApiErrorPayload(value: unknown): value is ApiErrorPayload {
  if (typeof value !== 'object' || value === null) {
    return false;
  }
  const error = value as Partial<ApiErrorPayload>;
  return (
    typeof error.code === 'string' &&
    typeof error.message === 'string' &&
    typeof error.request_id === 'string' &&
    typeof error.details === 'object' &&
    error.details !== null
  );
}
