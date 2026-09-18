import { ApiError, OfflineError } from '../api/client';

// The one place every page's catch block should turn a failed mutation
// into user-facing text. OfflineError's own message already says the
// device is offline and that the action needs a connection; ApiError's
// message is whatever the backend decided to say; only a genuinely
// unexpected error (a real bug, not connectivity or an API-level failure)
// falls back to the caller's own generic text.
export function describeActionError(err: unknown, fallback: string): string {
  if (err instanceof OfflineError) return err.message;
  if (err instanceof ApiError) return err.message;
  return fallback;
}
