import { useCallback, useEffect, useState } from 'react';

import {
  createPlatformStaff,
  listPlatformStaff,
  revokePlatformStaff,
  type PlatformStaffMember,
} from '../../api/platform';
import { describeActionError } from '../../lib/errors';

// Everyone can see the platform team; only a superadmin gets the create and
// revoke controls (the API enforces the same split -- platform:staff:manage).
export function PlatformStaffPanel({
  canManage,
  selfId,
  csrfToken,
  onChanged,
}: {
  canManage: boolean;
  selfId: string | undefined;
  csrfToken: string | null;
  onChanged: () => void;
}) {
  const [staff, setStaff] = useState<PlatformStaffMember[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [adding, setAdding] = useState(false);
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [role, setRole] = useState('support');
  const [formError, setFormError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const [revokingId, setRevokingId] = useState<string | null>(null);
  const [reason, setReason] = useState('');

  const load = useCallback(async () => {
    try {
      setStaff(await listPlatformStaff());
    } catch (err) {
      setLoadError(describeActionError(err, 'Could not load the team.'));
    }
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
  }, [load]);

  async function handleCreate() {
    if (!csrfToken) return;
    setSubmitting(true);
    setFormError(null);
    try {
      await createPlatformStaff({ name, email, password, role }, csrfToken);
      setAdding(false);
      setName('');
      setEmail('');
      setPassword('');
      setRole('support');
      await load();
      onChanged();
    } catch (err) {
      setFormError(describeActionError(err, 'Could not create this account.'));
    } finally {
      setSubmitting(false);
    }
  }

  async function handleRevoke(id: string) {
    if (!csrfToken) return;
    setSubmitting(true);
    setFormError(null);
    try {
      await revokePlatformStaff(id, reason, csrfToken);
      setRevokingId(null);
      setReason('');
      await load();
      onChanged();
    } catch (err) {
      setFormError(describeActionError(err, 'Could not revoke this account.'));
    } finally {
      setSubmitting(false);
    }
  }

  const inputClass =
    'rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] focus:outline-none focus:border-jb-ink/40';

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-xl font-medium">Team</h2>
        {canManage ? (
          !adding && (
            <button
              onClick={() => setAdding(true)}
              className="text-[13px] px-3 py-1.5 rounded-lg bg-jb-ink text-jb-cream"
            >
              Add staff
            </button>
          )
        ) : (
          <span className="text-[12px] text-jb-ink/40">
            Read-only. Only a superadmin can add or revoke staff.
          </span>
        )}
      </div>

      {adding && (
        <div className="rounded-xl border border-jb-ink/10 bg-white/70 p-4 mb-4">
          <div className="grid gap-2 sm:grid-cols-2">
            <input
              id="staff-name"
              className={inputClass}
              placeholder="Full name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <input
              id="staff-email"
              type="email"
              className={inputClass}
              placeholder="Email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
            <input
              id="staff-password"
              type="password"
              className={inputClass}
              placeholder="Password (min 8 characters)"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <select
              id="staff-role"
              className={inputClass}
              value={role}
              onChange={(e) => setRole(e.target.value)}
            >
              <option value="support">Support (read-only)</option>
              <option value="superadmin">Superadmin (full access)</option>
            </select>
          </div>
          <div className="flex items-center gap-2 mt-3">
            <button
              onClick={() => void handleCreate()}
              disabled={submitting || !name.trim() || !email.trim() || password.length < 8}
              className="text-[13px] px-4 py-2 rounded-lg bg-jb-ink text-jb-cream disabled:opacity-40"
            >
              {submitting ? 'Creating…' : 'Create account'}
            </button>
            <button
              onClick={() => {
                setAdding(false);
                setFormError(null);
              }}
              className="text-[13px] px-3 py-2 text-jb-ink/50 hover:text-jb-ink"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {(loadError || formError) && (
        <p role="alert" className="text-[13px] text-red-700 mb-3">
          {loadError ?? formError}
        </p>
      )}
      {staff === null && !loadError && <p className="text-[13px] text-jb-ink/45">Loading…</p>}

      {staff !== null && (
        <div className="rounded-xl border border-jb-ink/10 bg-white/70 divide-y divide-jb-ink/[0.06] overflow-hidden">
          {staff.map((s) => (
            <div key={s.id} className="px-4 py-3">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-[13px] text-jb-ink/85">
                    {s.name}
                    {s.id === selfId && <span className="text-jb-ink/40"> (you)</span>}
                  </div>
                  <div className="text-[11px] text-jb-ink/40 truncate">{s.email}</div>
                </div>
                <div className="flex items-center gap-3 shrink-0">
                  <span className="text-[11px] uppercase tracking-wide text-jb-ink/50">
                    {s.role}
                  </span>
                  {s.status === 'revoked' ? (
                    <span className="text-[11px] px-2 py-0.5 rounded-full bg-red-50 text-red-700">
                      revoked
                    </span>
                  ) : (
                    canManage &&
                    s.id !== selfId && (
                      <button
                        onClick={() => {
                          setRevokingId(s.id);
                          setReason('');
                          setFormError(null);
                        }}
                        className="text-[12px] px-3 py-1.5 rounded-lg border border-red-200 text-red-700 hover:bg-red-50"
                      >
                        Revoke
                      </button>
                    )
                  )}
                </div>
              </div>
              {revokingId === s.id && (
                <div className="flex items-center gap-2 mt-3">
                  <input
                    id={`revoke-reason-${s.id}`}
                    autoFocus
                    className={`${inputClass} flex-1`}
                    placeholder="Reason (recorded in the audit log)"
                    value={reason}
                    onChange={(e) => setReason(e.target.value)}
                  />
                  <button
                    onClick={() => void handleRevoke(s.id)}
                    disabled={submitting || !reason.trim()}
                    className="text-[13px] px-4 py-2 rounded-lg bg-jb-ink text-jb-cream disabled:opacity-40"
                  >
                    {submitting ? 'Working…' : 'Confirm'}
                  </button>
                  <button
                    onClick={() => setRevokingId(null)}
                    className="text-[13px] px-3 py-2 text-jb-ink/50 hover:text-jb-ink"
                  >
                    Cancel
                  </button>
                </div>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
