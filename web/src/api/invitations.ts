import { apiRequest } from './client';

export type InvitationRole = 'owner' | 'staff';
export type InvitationStatus = 'pending' | 'accepted' | 'revoked' | 'expired';

export interface Invitation {
  id: string;
  phone: string;
  email: string;
  role: InvitationRole;
  status: InvitationStatus;
  expiresAt: string;
  createdAt: string;
}

interface RawInvitation {
  id: string;
  phone?: string;
  email?: string;
  role: string;
  status: string;
  expires_at: string;
  created_at: string;
}

function toInvitation(raw: RawInvitation): Invitation {
  return {
    id: raw.id,
    phone: raw.phone ?? '',
    email: raw.email ?? '',
    role: raw.role as InvitationRole,
    status: raw.status as InvitationStatus,
    expiresAt: raw.expires_at,
    createdAt: raw.created_at,
  };
}

// createInvitation wraps POST /api/v1/invitations (members:manage). Sends
// via WhatsApp when phone is given, email otherwise -- invitations default
// to WhatsApp (docs/PHASE_INVITATIONS_WHATSAPP.md). Pass exactly one of
// phone/email.
export async function createInvitation(
  input: { phone?: string; email?: string; role: InvitationRole },
  businessId: string,
  csrfToken: string,
): Promise<Invitation> {
  const raw = await apiRequest<RawInvitation>('/api/v1/invitations', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
    body: JSON.stringify({ phone: input.phone ?? '', email: input.email ?? '', role: input.role }),
  });
  return toInvitation(raw);
}

// listInvitations wraps GET /api/v1/invitations (members:read).
export async function listInvitations(businessId: string): Promise<Invitation[]> {
  const raw = await apiRequest<RawInvitation[]>('/api/v1/invitations', {
    headers: { 'X-Business-ID': businessId },
  });
  return raw.map(toInvitation);
}

// revokeInvitation wraps POST /api/v1/invitations/{id}/revoke (members:manage).
export async function revokeInvitation(
  invitationId: string,
  businessId: string,
  csrfToken: string,
): Promise<Invitation> {
  const raw = await apiRequest<RawInvitation>(`/api/v1/invitations/${invitationId}/revoke`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken, 'X-Business-ID': businessId },
  });
  return toInvitation(raw);
}

// getInvitationByToken wraps GET /api/v1/invitations/{token}: public,
// unauthenticated -- the landing-page lookup an invitee hits before
// deciding "I'm new here" vs "I already have an account" (InviteAccept.tsx).
export async function getInvitationByToken(token: string): Promise<Invitation> {
  const raw = await apiRequest<RawInvitation>(`/api/v1/invitations/${encodeURIComponent(token)}`);
  return toInvitation(raw);
}

export interface LinkInvitationResult {
  verificationRequired: boolean;
  verificationId?: string;
  channel?: 'email' | 'whatsapp';
  invitation?: Invitation;
}

interface RawLinkResult {
  verification_required: boolean;
  verification_id?: string;
  channel?: string;
  invitation?: RawInvitation;
}

// linkInvitationToExistingUser wraps POST /api/v1/invitations/{token}/link:
// authenticated, not business-scoped -- flow E's "I already have an
// account" path. If the invitation's identifier already matches something
// on the caller's account, the membership is granted immediately
// (verificationRequired: false); otherwise a fresh identity_verifications
// challenge is started and its id returned for confirmIdentityVerification
// below to finish.
export async function linkInvitationToExistingUser(
  token: string,
  csrfToken: string,
): Promise<LinkInvitationResult> {
  const raw = await apiRequest<RawLinkResult>(
    `/api/v1/invitations/${encodeURIComponent(token)}/link`,
    {
      method: 'POST',
      headers: { 'X-CSRF-Token': csrfToken },
    },
  );
  return {
    verificationRequired: raw.verification_required,
    verificationId: raw.verification_id,
    channel: raw.channel as 'email' | 'whatsapp' | undefined,
    invitation: raw.invitation ? toInvitation(raw.invitation) : undefined,
  };
}

// confirmIdentityVerification wraps POST
// /api/v1/identity-verifications/{id}/confirm: authenticated. Passing
// invitationToken also finalizes that invitation's membership grant in the
// same request (see internal/httpapi/verification_handlers.go). Callers
// should follow a successful confirm with useSession().refresh() to pick
// up the new membership/identifier in session state.
export async function confirmIdentityVerification(
  verificationId: string,
  code: string,
  csrfToken: string,
  invitationToken?: string,
): Promise<void> {
  await apiRequest(`/api/v1/identity-verifications/${verificationId}/confirm`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ code, invitation_token: invitationToken ?? '' }),
  });
}
