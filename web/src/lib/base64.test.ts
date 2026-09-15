import { describe, expect, it } from 'vitest';

import { base64Decode, base64Encode, base64UrlDecode, base64UrlEncode } from './base64';

describe('base64UrlEncode/Decode', () => {
  it('round-trips arbitrary bytes', () => {
    const bytes = new Uint8Array([0, 1, 2, 253, 254, 255, 16, 32, 64, 128]);
    expect(base64UrlDecode(base64UrlEncode(bytes))).toEqual(bytes);
  });

  it('never contains padding or +/ characters', () => {
    const bytes = new Uint8Array(37).map((_, i) => i * 7);
    const encoded = base64UrlEncode(bytes);
    expect(encoded).not.toMatch(/[+/=]/);
  });

  it('decodes a known Go base64.RawURLEncoding value', () => {
    // echo -n 'hello' | base64 -> aGVsbG8= ; RawURLEncoding strips padding
    const decoded = base64UrlDecode('aGVsbG8');
    expect(new TextDecoder().decode(decoded)).toBe('hello');
  });
});

describe('base64Encode/Decode', () => {
  it('round-trips arbitrary bytes', () => {
    const bytes = new Uint8Array([0, 1, 2, 253, 254, 255]);
    expect(base64Decode(base64Encode(bytes))).toEqual(bytes);
  });

  it('matches a known standard base64 value', () => {
    expect(base64Encode(new TextEncoder().encode('hello'))).toBe('aGVsbG8=');
  });
});
