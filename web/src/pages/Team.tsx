import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import {
  createInvitation,
  listInvitations,
  revokeInvitation,
  type Invitation,
  type InvitationRole,
} from '../api/invitations';
import { ApiError } from '../api/client';
import { EmailIcon, WhatsAppIcon } from '../components/ChannelIcon';
import { PhoneInput } from '../components/PhoneInput';
import { describeActionError } from '../lib/errors';
import { useSession } from '../lib/session';

function statusLabel(status: Invitation['status']): string {
  switch (status) {
    case 'pending':
      return 'Pending';
    case 'accepted':
      return 'Accepted';
    case 'revoked':
      return 'Revoked';
    case 'expired':
      return 'Expired';
  }
}

function statusColorClass(status: Invitation['status']): string {
  switch (status) {
    case 'pending':
      return 'text-blue-600';
    case 'accepted':
      return 'text-green-700';
    case 'revoked':
      return 'text-red-700';
    case 'expired':
      return 'text-jb-ink/40';
  }
}

// Owner-facing staff invitations (docs/PHASE_INVITATIONS_WHATSAPP.md).
// Invites default to WhatsApp (phone) with email as the fallback channel --
// mirrors internal/invitations.Service.Create's own default. Only Owners
// can send/revoke (members:manage); Staff can view the list read-only
// (members:read), matching the backend capability split exactly.
export function Team() {
  const { csrfToken, selectedBusinessId, memberships } = useSession();
  const isOwner = memberships.find((m) => m.businessId === selectedBusinessId)?.role === 'owner';

  const [invitations, setInvitations] = useState<Invitation[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);

  const [channel, setChannel] = useState<'whatsapp' | 'email'>('whatsapp');
  const [identifier, setIdentifier] = useState('');
  // Bumped whenever identifier is reset back to '' from code (after a
  // successful send, or a channel switch) -- remounts PhoneInput so its own
  // internal country/local-number state clears along with it, since it
  // isn't a controlled input.
  const [phoneInputKey, setPhoneInputKey] = useState(0);
  const [role, setRole] = useState<InvitationRole>('staff');
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  useEffect(() => {
    if (!selectedBusinessId) return;
    listInvitations(selectedBusinessId)
      .then(setInvitations)
      .catch((err: unknown) =>
        setLoadError(describeActionError(err, 'Could not load invitations.')),
      );
  }, [selectedBusinessId]);

  async function handleInvite(e: React.FormEvent) {
    e.preventDefault();
    if (!selectedBusinessId || !csrfToken) return;
    setSubmitting(true);
    setFormError(null);
    try {
      const inv = await createInvitation(
        channel === 'whatsapp' ? { phone: identifier, role } : { email: identifier, role },
        selectedBusinessId,
        csrfToken,
      );
      setInvitations((prev) => [inv, ...(prev ?? [])]);
      setIdentifier('');
      setPhoneInputKey((k) => k + 1);
    } catch (err) {
      if (err instanceof ApiError && (err.status === 409 || err.status === 422)) {
        setFormError(err.message);
      } else {
        setFormError(describeActionError(err, 'Could not send this invitation.'));
      }
    } finally {
      setSubmitting(false);
    }
  }

  async function handleRevoke(id: string) {
    if (!selectedBusinessId || !csrfToken) return;
    setBusyId(id);
    try {
      const updated = await revokeInvitation(id, selectedBusinessId, csrfToken);
      setInvitations((prev) => (prev ?? []).map((i) => (i.id === id ? updated : i)));
    } catch (err) {
      setLoadError(describeActionError(err, 'Could not revoke this invitation.'));
    } finally {
      setBusyId(null);
    }
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-16">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center gap-4 mb-6">
          <Link
            to="/dashboard"
            aria-label="Back"
            className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none"
          >
            ←
          </Link>
          <div>
            <div className="text-[11px] text-jb-ink/45">Dashboard</div>
            <h1 className="text-lg font-medium">Team</h1>
          </div>
        </div>

        {loadError && <p className="text-[13px] text-red-700 mb-3">{loadError}</p>}

        {isOwner && (
          <form
            onSubmit={(e) => void handleInvite(e)}
            className="rounded-xl border border-jb-ink/10 bg-white/60 p-4 mb-6"
          >
            <div className="flex gap-2 mb-3">
              <button
                type="button"
                onClick={() => {
                  setChannel('whatsapp');
                  setIdentifier('');
                }}
                className={`flex-1 flex items-center justify-center gap-1.5 rounded-lg border py-2 text-[13px] font-medium transition-colors ${
                  channel === 'whatsapp'
                    ? 'border-jb-ink bg-jb-ink text-jb-cream'
                    : 'border-jb-ink/15 bg-white text-jb-ink/60'
                }`}
              >
                <WhatsAppIcon className="w-4 h-4" />
                WhatsApp
              </button>
              <button
                type="button"
                onClick={() => {
                  setChannel('email');
                  setIdentifier('');
                }}
                className={`flex-1 flex items-center justify-center gap-1.5 rounded-lg border py-2 text-[13px] font-medium transition-colors ${
                  channel === 'email'
                    ? 'border-jb-ink bg-jb-ink text-jb-cream'
                    : 'border-jb-ink/15 bg-white text-jb-ink/60'
                }`}
              >
                <EmailIcon className="w-4 h-4" />
                Email
              </button>
            </div>

            {channel === 'whatsapp' ? (
              <div className="mb-3">
                <PhoneInput key={phoneInputKey} required onChange={setIdentifier} />
              </div>
            ) : (
              <>
                <input
                  type="email"
                  required
                  placeholder="ada@example.com"
                  value={identifier}
                  onChange={(e) => setIdentifier(e.target.value)}
                  className="w-full rounded-lg border border-jb-ink/15 bg-white px-3.5 py-2.5 text-[14px] text-jb-ink placeholder:text-jb-ink/30 focus:outline-none focus:border-jb-ink/40 transition-colors mb-3"
                />
                <p className="text-[12px] text-jb-ink/45 mb-3 leading-snug">
                  Invite emails sometimes land in spam — kindly ask your invitee to check there if
                  they don&apos;t see it.
                </p>
              </>
            )}

            <div className="flex gap-2 mb-3">
              <button
                type="button"
                onClick={() => setRole('staff')}
                className={`flex-1 rounded-lg border py-2 text-[13px] font-medium transition-colors ${
                  role === 'staff'
                    ? 'border-jb-ink bg-jb-ink text-jb-cream'
                    : 'border-jb-ink/15 bg-white text-jb-ink/60'
                }`}
              >
                Staff
              </button>
              <button
                type="button"
                onClick={() => setRole('owner')}
                className={`flex-1 rounded-lg border py-2 text-[13px] font-medium transition-colors ${
                  role === 'owner'
                    ? 'border-jb-ink bg-jb-ink text-jb-cream'
                    : 'border-jb-ink/15 bg-white text-jb-ink/60'
                }`}
              >
                Owner
              </button>
            </div>

            {formError && <p className="text-[12.5px] text-red-700 mb-2">{formError}</p>}

            <button
              type="submit"
              disabled={submitting || !identifier}
              className="w-full rounded-lg bg-jb-ink text-jb-cream text-[14px] font-medium py-2.5 hover:bg-jb-green transition-colors active:scale-[0.98] disabled:opacity-40"
            >
              {submitting ? 'Sending…' : 'Send invitation'}
            </button>
          </form>
        )}

        <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
          {invitations === null && (
            <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">Loading…</p>
          )}
          {invitations !== null && invitations.length === 0 && (
            <p className="text-[13px] text-jb-ink/40 px-4 py-3.5">No invitations yet.</p>
          )}
          {(invitations ?? []).map((inv) => (
            <div key={inv.id} className="px-4 py-3.5 flex items-center justify-between gap-3">
              <div className="min-w-0">
                <div className="text-[13px] text-jb-ink/80 truncate">{inv.phone || inv.email}</div>
                <div className="text-[11px] text-jb-ink/40">
                  {inv.role} ·{' '}
                  <span className={statusColorClass(inv.status)}>{statusLabel(inv.status)}</span>
                </div>
              </div>
              {isOwner && inv.status === 'pending' && (
                <button
                  onClick={() => void handleRevoke(inv.id)}
                  disabled={busyId === inv.id}
                  className="shrink-0 text-[12.5px] text-red-700 hover:text-red-800 transition-colors disabled:opacity-40"
                >
                  {busyId === inv.id ? 'Revoking…' : 'Revoke'}
                </button>
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
