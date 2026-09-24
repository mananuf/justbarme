import { apiRequest } from './client';

export interface PlatformBusiness {
  id: string;
  name: string;
  status: string;
  timezone: string;
  currency: string;
  created_at: string;
}

export interface PlatformStats {
  total_businesses: number;
  active_businesses: number;
  suspended_businesses: number;
  new_businesses_last_7d: number;
  total_users: number;
}

export interface PlatformActivityHeadline {
  type: string;
  occurred_at: string;
  summary: string;
  amount_kobo: number;
}

// Aggregate-only oversight summary of one business -- no line items, no
// customer or staff detail beyond role counts. Every fetch of this is
// written to the platform audit log server-side.
export interface PlatformBusinessActivity {
  business: PlatformBusiness;
  sales_today_kobo: number;
  sales_today_count: number;
  items_sold_today: number;
  sales_7d_kobo: number;
  sales_7d_count: number;
  outstanding_kobo: number;
  outstanding_bills: number;
  alerts_count: number;
  tracked_variants: number;
  out_of_stock: number;
  negative_stock: number;
  owners: number;
  staff: number;
  last_activity_at?: string;
  recent_activity: PlatformActivityHeadline[];
}

export interface PlatformStaffMember {
  id: string;
  name: string;
  email: string;
  role: string;
  status: string;
  created_at: string;
}

export function getPlatformStats(): Promise<PlatformStats> {
  return apiRequest<PlatformStats>('/api/v1/platform/stats');
}

export function getPlatformBusinessActivity(businessId: string): Promise<PlatformBusinessActivity> {
  return apiRequest<PlatformBusinessActivity>(`/api/v1/platform/businesses/${businessId}/activity`);
}

export function listPlatformStaff(): Promise<PlatformStaffMember[]> {
  return apiRequest<PlatformStaffMember[]>('/api/v1/platform/staff');
}

export function createPlatformStaff(
  input: { name: string; email: string; password: string; role: string },
  csrfToken: string,
): Promise<PlatformStaffMember> {
  return apiRequest<PlatformStaffMember>('/api/v1/platform/staff', {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify(input),
  });
}

export function revokePlatformStaff(
  staffId: string,
  reason: string,
  csrfToken: string,
): Promise<PlatformStaffMember> {
  return apiRequest<PlatformStaffMember>(`/api/v1/platform/staff/${staffId}/revoke`, {
    method: 'POST',
    headers: { 'X-CSRF-Token': csrfToken },
    body: JSON.stringify({ reason }),
  });
}
