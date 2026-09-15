import { afterEach, describe, expect, it, vi } from 'vitest';

import { ApiError, apiRequest } from './client';

function response(body: unknown, init: ResponseInit = {}) {
  return new Response(body == null ? null : JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json', ...init.headers },
    ...init,
  });
}

afterEach(() => vi.unstubAllGlobals());

describe('apiRequest', () => {
  it('returns data and sends same-origin credentials', async () => {
    const fetchMock = vi.fn().mockResolvedValue(response({ data: { status: 'ready' } }));
    vi.stubGlobal('fetch', fetchMock);

    await expect(apiRequest<{ status: string }>('/api/v1/health/ready')).resolves.toEqual({
      status: 'ready',
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/health/ready',
      expect.objectContaining({ credentials: 'same-origin' }),
    );
  });

  it('throws the stable API error envelope', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        response(
          {
            error: {
              code: 'VALIDATION_FAILED',
              message: 'Invalid input provided.',
              request_id: 'request-1',
              details: { email: 'invalid' },
            },
          },
          { status: 422 },
        ),
      ),
    );

    const error = await apiRequest('/api/v1/test').catch((caught: unknown) => caught);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 422, code: 'VALIDATION_FAILED', requestId: 'request-1' });
  });

  it('normalizes malformed non-success responses', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response('bad gateway', { status: 502, headers: { 'Content-Type': 'text/plain' } }),
        ),
    );

    await expect(apiRequest('/api/v1/test')).rejects.toMatchObject({
      status: 502,
      code: 'UNEXPECTED_RESPONSE',
    });
  });

  it('rejects successful responses without data', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(response({ meta: {} })));
    await expect(apiRequest('/api/v1/test')).rejects.toMatchObject({ code: 'UNEXPECTED_RESPONSE' });
  });

  it('supports 204 responses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })));
    await expect(apiRequest<void>('/api/v1/test', { method: 'DELETE' })).resolves.toBeUndefined();
  });
});
