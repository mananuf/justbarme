import { Fragment, useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { apiRequest } from '../../api/client';
import { Logo } from '../../components/Logo';
import { describeActionError } from '../../lib/errors';
import { usePlatformSession } from '../../lib/platformSession';

interface PlatformBusiness {
  id: string;
  name: string;
  status: string;
  timezone: string;
  currency: string;
  created_at: string;
}

interface PlatformAuditEntry {
  id: string;
  staff_id: string;
  action: string;
  target_business_id?: string;
  reason?: string;
  created_at: string;
}

function StatusBadge({ status }: { status: string }) {
  const active = status === 'active';
  return (
    <span
      className={`inline-flex items-center gap-1.5 text-[11px] font-medium px-2 py-0.5 rounded-full ${
        active ? 'bg-emerald-50 text-emerald-700' : 'bg-jb-gold/15 text-[#7a5b1f]'
      }`}
    >
      <span
        className={`w-1.5 h-1.5 rounded-full ${active ? 'bg-emerald-500' : 'bg-jb-gold'}`}
        aria-hidden="true"
      />
      {status}
    </span>
  );
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

function formatDateTime(iso: string): string {
  return new Date(iso).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' });
}

function describeAction(action: string): string {
  switch (action) {
    case 'login':
      return 'Signed in';
    case 'business.suspended':
      return 'Suspended a business';
    case 'business.reactivated':
      return 'Reactivated a business';
    default:
      return action;
  }
}

export function PlatformDashboard() {
  const { staff, csrfToken, logout } = usePlatformSession();
  const navigate = useNavigate();

  const [businesses, setBusinesses] = useState<PlatformBusiness[] | null>(null);
  const [auditLog, setAuditLog] = useState<PlatformAuditEntry[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [pendingAction, setPendingAction] = useState<{
    businessId: string;
    suspend: boolean;
  } | null>(null);
  const [reason, setReason] = useState('');
  const [actionSubmitting, setActionSubmitting] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);
  const [showAuditLog, setShowAuditLog] = useState(false);

  const canSuspend = staff?.role === 'superadmin';

  const loadBusinesses = useCallback(async () => {
    try {
      const data = await apiRequest<PlatformBusiness[]>('/api/v1/platform/businesses');
      setBusinesses(data);
    } catch (err) {
      setLoadError(describeActionError(err, 'Could not load businesses.'));
    }
  }, []);

  const loadAuditLog = useCallback(async () => {
    try {
      const data = await apiRequest<PlatformAuditEntry[]>('/api/v1/platform/audit-log');
      setAuditLog(data);
    } catch {
      // Audit log is secondary; a failure here shouldn't block the page.
    }
  }, []);

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void loadBusinesses();
    void loadAuditLog();
  }, [loadBusinesses, loadAuditLog]);

  async function handleLogout() {
    await logout();
    void navigate('/platform/login', { replace: true });
  }

  function startAction(businessId: string, suspend: boolean) {
    setPendingAction({ businessId, suspend });
    setReason('');
    setActionError(null);
  }

  async function confirmAction() {
    if (!pendingAction || !csrfToken) return;
    setActionSubmitting(true);
    setActionError(null);
    try {
      const path = `/api/v1/platform/businesses/${pendingAction.businessId}/${pendingAction.suspend ? 'suspend' : 'reactivate'}`;
      const updated = await apiRequest<PlatformBusiness>(path, {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrfToken },
        body: JSON.stringify({ reason }),
      });
      setBusinesses((prev) => prev?.map((b) => (b.id === updated.id ? updated : b)) ?? prev);
      setPendingAction(null);
      void loadAuditLog();
    } catch (err) {
      setActionError(describeActionError(err, 'Could not complete this action.'));
    } finally {
      setActionSubmitting(false);
    }
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink">
      <header className="border-b border-jb-ink/10 bg-white/60">
        <div className="max-w-4xl mx-auto px-6 py-4 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Logo className="w-5 h-5 text-jb-ink/70" />
            <span className="font-pixel text-[10px] tracking-[0.2em] text-jb-ink/50">
              JUSTBARME PLATFORM
            </span>
          </div>
          <div className="flex items-center gap-3">
            <div className="text-right">
              <div className="text-[13px] text-jb-ink/80">{staff?.name}</div>
              <div className="text-[11px] text-jb-ink/40 uppercase tracking-wide">
                {staff?.role}
              </div>
            </div>
            <button
              onClick={() => void handleLogout()}
              className="text-[13px] text-jb-ink/50 hover:text-jb-ink px-3 py-1.5 rounded-lg border border-jb-ink/10 transition-colors"
            >
              Sign out
            </button>
          </div>
        </div>
      </header>

      <main className="max-w-4xl mx-auto px-6 py-8">
        <div className="flex items-center justify-between mb-4">
          <h1 className="text-xl font-medium">Businesses</h1>
          {!canSuspend && (
            <span className="text-[12px] text-jb-ink/40">
              Read-only. The support role can&apos;t suspend or reactivate.
            </span>
          )}
        </div>

        {loadError && (
          <p role="alert" className="text-[13px] text-red-700 mb-4">
            {loadError}
          </p>
        )}

        {businesses === null && !loadError && (
          <p className="text-[13px] text-jb-ink/45">Loading…</p>
        )}

        {businesses !== null && businesses.length === 0 && (
          <p className="text-[13px] text-jb-ink/45">No businesses on the service yet.</p>
        )}

        {businesses !== null && businesses.length > 0 && (
          <div className="rounded-xl border border-jb-ink/10 bg-white/70 overflow-hidden">
            <table className="w-full text-left text-[13px]">
              <thead>
                <tr className="border-b border-jb-ink/10 text-[11px] text-jb-ink/45 uppercase tracking-wide">
                  <th className="px-4 py-3 font-medium">Business</th>
                  <th className="px-4 py-3 font-medium">Status</th>
                  <th className="px-4 py-3 font-medium">Currency</th>
                  <th className="px-4 py-3 font-medium">Created</th>
                  {canSuspend && <th className="px-4 py-3 font-medium text-right">Action</th>}
                </tr>
              </thead>
              <tbody>
                {businesses.map((b) => (
                  <Fragment key={b.id}>
                    <tr className="border-b border-jb-ink/[0.06] last:border-0">
                      <td className="px-4 py-3">
                        <div className="text-jb-ink/85">{b.name}</div>
                        <div className="text-[11px] text-jb-ink/35">{b.timezone}</div>
                      </td>
                      <td className="px-4 py-3">
                        <StatusBadge status={b.status} />
                      </td>
                      <td className="px-4 py-3 text-jb-ink/60">{b.currency}</td>
                      <td className="px-4 py-3 text-jb-ink/60">{formatDate(b.created_at)}</td>
                      {canSuspend && (
                        <td className="px-4 py-3 text-right">
                          <button
                            onClick={() => startAction(b.id, b.status === 'active')}
                            className={`text-[12px] px-3 py-1.5 rounded-lg border transition-colors ${
                              b.status === 'active'
                                ? 'border-red-200 text-red-700 hover:bg-red-50'
                                : 'border-emerald-200 text-emerald-700 hover:bg-emerald-50'
                            }`}
                          >
                            {b.status === 'active' ? 'Suspend' : 'Reactivate'}
                          </button>
                        </td>
                      )}
                    </tr>
                    {pendingAction?.businessId === b.id && (
                      <tr key={`${b.id}-action`} className="bg-jb-ink/[0.03]">
                        <td colSpan={canSuspend ? 5 : 4} className="px-4 py-4">
                          <p className="text-[13px] text-jb-ink/70 mb-2">
                            {pendingAction.suspend ? 'Suspend' : 'Reactivate'}{' '}
                            <strong>{b.name}</strong>. A reason is required, and it&apos;s recorded
                            in the audit log.
                          </p>
                          <div className="flex items-center gap-2">
                            <input
                              type="text"
                              autoFocus
                              value={reason}
                              onChange={(e) => setReason(e.target.value)}
                              placeholder="e.g. non-payment, abuse report, owner request"
                              className="flex-1 rounded-lg border border-jb-ink/15 bg-white px-3 py-2 text-[13px] focus:outline-none focus:border-jb-ink/40"
                            />
                            <button
                              onClick={() => void confirmAction()}
                              disabled={actionSubmitting || !reason.trim()}
                              className="text-[13px] px-4 py-2 rounded-lg bg-jb-ink text-jb-cream disabled:opacity-40 transition-colors"
                            >
                              {actionSubmitting ? 'Working…' : 'Confirm'}
                            </button>
                            <button
                              onClick={() => setPendingAction(null)}
                              className="text-[13px] px-3 py-2 text-jb-ink/50 hover:text-jb-ink"
                            >
                              Cancel
                            </button>
                          </div>
                          {actionError && (
                            <p role="alert" className="text-[12px] text-red-700 mt-2">
                              {actionError}
                            </p>
                          )}
                        </td>
                      </tr>
                    )}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        )}

        <div className="mt-8">
          <button
            onClick={() => setShowAuditLog((v) => !v)}
            className="text-[13px] text-jb-ink/60 hover:text-jb-ink flex items-center gap-1.5"
          >
            {showAuditLog ? 'Hide' : 'Show'} recent activity
            <span className={`transition-transform ${showAuditLog ? 'rotate-180' : ''}`}>⌄</span>
          </button>

          {showAuditLog && (
            <div className="mt-3 rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
              {auditLog === null && (
                <p className="px-4 py-3 text-[13px] text-jb-ink/45">Loading…</p>
              )}
              {auditLog !== null && auditLog.length === 0 && (
                <p className="px-4 py-3 text-[13px] text-jb-ink/45">No activity recorded yet.</p>
              )}
              {auditLog?.map((entry) => (
                <div key={entry.id} className="px-4 py-3 flex items-center justify-between gap-4">
                  <div>
                    <div className="text-[13px] text-jb-ink/80">{describeAction(entry.action)}</div>
                    {entry.reason && (
                      <div className="text-[11px] text-jb-ink/40">{entry.reason}</div>
                    )}
                  </div>
                  <div className="text-[11px] text-jb-ink/40 shrink-0">
                    {formatDateTime(entry.created_at)}
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </main>
    </div>
  );
}
