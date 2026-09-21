import { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';

import { getDashboard, type DashboardData } from '../api/dashboard';
import { AppBottomNav } from '../components/AppBottomNav';
import { DashboardTour } from '../components/DashboardTour';
import { Logo } from '../components/Logo';
import { hasSeenDashboardTour } from '../lib/dashboardTour';
import { StatusBadge } from '../components/StatusBadge';
import { useConnectivity } from '../hooks/useConnectivity';
import { useServiceHealth } from '../hooks/useServiceHealth';
import { db } from '../lib/db';
import { checkLease } from '../lib/lease';
import { useSession } from '../lib/session';

function formatNaira(kobo: number): string {
  return (kobo / 100).toLocaleString();
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' });
}

// Based on the device's own local time (this is a greeting on someone's own
// phone, not a financial boundary like GET /sales/summary's business-
// timezone "today" -- no need to fetch the business's timezone for this).
function timeBasedGreeting(): string {
  const hour = new Date().getHours();
  if (hour < 12) return 'Good morning';
  if (hour < 17) return 'Good afternoon';
  return 'Good evening';
}

// Shared by the inline dropdown (the "More" row on this page) and the
// popup card (the bottom nav's ••• dots icon) -- one list of links, two
// presentations, rather than duplicating six Links twice.
function MoreMenuItems({
  isOwner,
  leaseStatus,
  userIdentifier,
  onNavigate,
  onLogout,
}: {
  isOwner: boolean;
  leaseStatus: LeaseStatus;
  userIdentifier: string;
  onNavigate?: () => void;
  onLogout: () => void;
}) {
  return (
    <div className="rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
      <Link
        to="/dashboard/expenses"
        onClick={onNavigate}
        className="px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
      >
        <span className="text-[13px] text-jb-ink/75">Expenses</span>
        <span className="text-[11px] text-jb-ink/40">Ice, transport, fuel and supplies</span>
      </Link>
      {isOwner && (
        <Link
          to="/dashboard/reports"
          onClick={onNavigate}
          className="px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
        >
          <span className="text-[13px] text-jb-ink/75">Reports</span>
          <span className="text-[11px] text-jb-ink/40">Sales, stock and expense summaries</span>
        </Link>
      )}
      {isOwner && (
        <Link
          to="/dashboard/activity"
          onClick={onNavigate}
          className="px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
        >
          <span className="text-[13px] text-jb-ink/75">Activity</span>
          <span className="text-[11px] text-jb-ink/40">Everything the team has done today</span>
        </Link>
      )}
      <Link
        to="/dashboard/team"
        onClick={onNavigate}
        className="px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
      >
        <span className="text-[13px] text-jb-ink/75">Team</span>
        <span className="text-[11px] text-jb-ink/40">Invite staff, manage access</span>
      </Link>
      {isOwner && (
        <Link
          to="/dashboard/reviews"
          onClick={onNavigate}
          className="px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
        >
          <span className="text-[13px] text-jb-ink/75">Reviews</span>
          <span className="text-[11px] text-jb-ink/40">
            Sales flagged for a stale price or deactivated item
          </span>
        </Link>
      )}
      {isOwner && (
        <Link
          to="/dashboard/settings"
          onClick={onNavigate}
          className="px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
        >
          <span className="text-[13px] text-jb-ink/75">Settings</span>
          <span className="text-[11px] text-jb-ink/40">Branding, logo, and catalogue</span>
        </Link>
      )}
      <Link
        to="/install"
        onClick={onNavigate}
        className="px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
      >
        <span className="text-[13px] text-jb-ink/75">Install justbarme</span>
        <span className="text-[11px] text-jb-ink/40">
          {leaseStatus.kind === 'valid'
            ? `Works offline until ${new Date(leaseStatus.expiresAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`
            : 'Add justbarme to your home screen'}
        </span>
      </Link>
      <button
        onClick={onLogout}
        className="w-full text-left px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
      >
        <span className="text-[13px] text-jb-ink/75">Sign out</span>
        <span className="text-[11px] text-jb-ink/40">{userIdentifier}</span>
      </button>
    </div>
  );
}

type LeaseStatus = { kind: 'none' } | { kind: 'valid'; expiresAt: string } | { kind: 'expired' };

function useLeaseStatus(): LeaseStatus {
  const [status, setStatus] = useState<LeaseStatus>({ kind: 'none' });
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const record = await db.device.get('current');
      if (!record?.lease) return;
      const result = await checkLease(
        record.lease,
        import.meta.env.VITE_OFFLINE_SIGNING_PUBLIC_KEY ?? '',
      );
      if (cancelled || !result.payload) return;
      setStatus(
        result.expired
          ? { kind: 'expired' }
          : { kind: 'valid', expiresAt: result.payload.expires_at },
      );
    })();
    return () => {
      cancelled = true;
    };
  }, []);
  return status;
}

export function Dashboard() {
  const [showGuide, setShowGuide] = useState(true);
  // Two separate triggers, two presentations of the same MoreMenuItems
  // list: the in-page "More" row expands it as a normal inline dropdown
  // (moreOpen); the bottom nav's ••• dots icon instead pops it up as a
  // card overlay (moreMenuPopupOpen), since that's a more typical
  // "menu button" interaction than scrolling to an inline section.
  const [moreOpen, setMoreOpen] = useState(false);
  const [moreMenuPopupOpen, setMoreMenuPopupOpen] = useState(false);
  const [showTour, setShowTour] = useState(() => !hasSeenDashboardTour());
  const isOnline = useConnectivity();
  const serviceHealth = useServiceHealth(isOnline);
  const { user, memberships, selectedBusinessId, logout } = useSession();
  const navigate = useNavigate();
  const location = useLocation();
  const leaseStatus = useLeaseStatus();

  const [dashboard, setDashboard] = useState<DashboardData | null>(null);

  // AppBottomNav's "More" tab (the ••• dots icon) links to /dashboard#more
  // -- from any other page that's a real navigation, but arriving here
  // never actually opened anything (the button looked "dead").
  useEffect(() => {
    if (location.hash !== '#more') return;
    // Synchronizing with an external system (the URL hash), the
    // documented exception to this rule -- not state derived from props.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setMoreMenuPopupOpen(true);
  }, [location.hash]);

  useEffect(() => {
    if (!selectedBusinessId) return;
    let cancelled = false;
    // One round trip (docs/PHASE_EXPENSES_DASHBOARD_ACTIVITY_REPORTS.md
    // §3) replacing the three separate calls this used to make. The
    // response is capability-aware per section -- a Staff viewer gets
    // only the fields their role can see (e.g. just outstandingKobo)
    // rather than the whole call failing, which is what used to happen
    // silently here.
    getDashboard(selectedBusinessId)
      .then((d) => {
        if (cancelled) return;
        setDashboard(d);
      })
      .catch(() => {
        // Best-effort: the dashboard still renders fine with these cards
        // simply empty if the call fails (e.g. offline).
      });
    return () => {
      cancelled = true;
    };
  }, [selectedBusinessId]);

  const business = memberships.find((m) => m.businessId === selectedBusinessId) ?? memberships[0];

  async function handleLogout() {
    await logout();
    void navigate('/login', { replace: true });
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-28">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center justify-between gap-3 mb-5">
          <div className="flex items-center gap-2.5 min-w-0">
            <div className="w-8 h-8 rounded-lg bg-jb-green-dark flex items-center justify-center shrink-0">
              <Logo className="w-5 h-5 text-jb-cream" />
            </div>
            <div className="min-w-0">
              <div className="text-[12px] text-jb-ink/45 truncate">
                {business?.businessName ?? 'justbarme'}
              </div>
              <h1 className="text-xl font-medium truncate">
                {timeBasedGreeting()}
                {user ? `, ${user.name.split(' ')[0]}` : ''} 👋
              </h1>
            </div>
          </div>
          <div className="shrink-0">
            <StatusBadge status={serviceHealth} />
          </div>
        </div>

        {leaseStatus.kind === 'expired' && (
          <div
            className="mb-5 rounded-xl border border-jb-gold/30 bg-jb-gold/10 px-4 py-3"
            role="alert"
          >
            <p className="text-[12.5px] text-[#7a5b1f]">
              Offline access on this device has expired. Connect to the internet to renew it.
            </p>
          </div>
        )}

        {showGuide && (
          <div className="mb-5 rounded-xl border border-jb-ink/10 bg-white/70 px-4 py-3 flex items-start justify-between gap-3">
            <p className="text-[12.5px] text-jb-ink/60 leading-relaxed">
              Start by recording your first sale. You can customize your products and settings
              anytime.
            </p>
            <button
              onClick={() => setShowGuide(false)}
              className="text-jb-ink/30 hover:text-jb-ink text-sm shrink-0"
            >
              ✕
            </button>
          </div>
        )}

        <div className="rounded-2xl bg-jb-green-dark text-jb-cream p-5 mb-4">
          <div className="text-[11px] text-jb-cream/60 tracking-wide">TODAY&apos;S SALES</div>
          <div
            className="text-[34px] font-light mt-1"
            style={{ fontFamily: 'var(--font-display)' }}
          >
            {dashboard?.todayTotalKobo !== null && dashboard?.todayTotalKobo !== undefined
              ? `₦${formatNaira(dashboard.todayTotalKobo)}`
              : '—'}
          </div>
          <div className="flex gap-4 mt-3 text-[12px] text-jb-cream/70">
            <span>
              {dashboard?.todaySaleCount !== null && dashboard?.todaySaleCount !== undefined
                ? `${dashboard.todaySaleCount} sale${dashboard.todaySaleCount === 1 ? '' : 's'}`
                : 'Loading…'}
            </span>
            {dashboard?.itemsSoldToday !== null && dashboard?.itemsSoldToday !== undefined && (
              <span>{dashboard.itemsSoldToday} items sold</span>
            )}
          </div>
        </div>

        <Link
          id="quick-sell"
          to="/dashboard/sell"
          className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 flex items-center justify-center gap-2 active:scale-[0.98] transition-transform mb-4"
        >
          <span className="text-xl leading-none">+</span> Quick Sell
        </Link>

        <div id="stock" className="grid grid-cols-2 gap-3 mb-4 scroll-mt-20">
          <Link
            to="/dashboard/reviews"
            className="rounded-xl bg-white/70 border border-jb-ink/[0.08] p-4 active:scale-[0.98] transition-transform"
          >
            <div className="text-[11px] text-jb-ink/45">Alerts</div>
            <div className="text-[15px] font-medium text-jb-ink mt-1">
              {dashboard?.alertsCount !== null && dashboard?.alertsCount !== undefined
                ? `${dashboard.alertsCount} open`
                : '—'}
            </div>
            <div className="mt-2 text-[10px] text-jb-ink/40">Reviews &amp; adjustments</div>
          </Link>
          <Link
            id="bills"
            to="/dashboard/tabs"
            className="rounded-xl bg-white/70 border border-jb-ink/[0.08] p-4 scroll-mt-20 active:scale-[0.98] transition-transform"
          >
            <div className="text-[11px] text-jb-ink/45">Outstanding</div>
            <div className="text-[15px] font-medium text-jb-ink mt-1">
              {dashboard?.outstandingKobo !== null && dashboard?.outstandingKobo !== undefined
                ? `₦${formatNaira(dashboard.outstandingKobo)}`
                : '—'}
            </div>
          </Link>
        </div>

        {dashboard?.todayExpensesKobo !== null && dashboard?.todayExpensesKobo !== undefined && (
          <Link
            to="/dashboard/expenses"
            className="w-full flex items-center justify-between rounded-xl bg-white/70 border border-jb-ink/[0.08] p-4 mb-4 active:scale-[0.98] transition-transform"
          >
            <span className="text-[11px] text-jb-ink/45">Today&apos;s expenses</span>
            <span className="text-[13px] font-medium text-jb-ink">
              ₦{formatNaira(dashboard.todayExpensesKobo)}
            </span>
          </Link>
        )}

        <div className="mb-4">
          <div className="flex items-center justify-between mb-2">
            <span className="text-[11px] text-jb-ink/45 tracking-wide">RECENT ACTIVITY</span>
            {dashboard?.recentActivity && (
              <Link
                to="/dashboard/activity"
                className="text-[11px] text-jb-ink/40 hover:text-jb-ink"
              >
                See all
              </Link>
            )}
          </div>
          <div className="space-y-2">
            {dashboard === null && <p className="text-[12.5px] text-jb-ink/40 px-1">Loading…</p>}
            {dashboard !== null && !dashboard.recentActivity.length && (
              <p className="text-[12.5px] text-jb-ink/40 px-1">Nothing recorded yet.</p>
            )}
            {(dashboard?.recentActivity ?? []).map((entry) => (
              <div
                key={entry.id}
                className="flex items-center justify-between rounded-xl px-3.5 py-3 bg-white/60 border border-jb-ink/[0.05]"
              >
                <div>
                  <div className="text-[13px] text-jb-ink/80">
                    {entry.actorName && <span className="font-semibold">{entry.actorName}</span>}{' '}
                    {entry.summary}
                  </div>
                  <div className="text-[11px] text-jb-ink/40">{formatTime(entry.occurredAt)}</div>
                </div>
                <div
                  className={`text-[12.5px] ${entry.amountKobo < 0 ? 'text-red-700/70' : 'text-jb-ink/55'}`}
                >
                  {entry.amountKobo !== 0 && (
                    <>
                      {entry.amountKobo > 0 ? '+' : '−'}₦{formatNaira(Math.abs(entry.amountKobo))}
                    </>
                  )}
                </div>
              </div>
            ))}
          </div>
        </div>

        <div id="more" className="scroll-mt-20">
          <button
            onClick={() => setMoreOpen((v) => !v)}
            className="w-full flex items-center justify-between rounded-xl border border-jb-ink/10 bg-white/60 px-4 py-3.5 text-[13px] text-jb-ink/60"
          >
            More
            <span className={`transition-transform ${moreOpen ? 'rotate-180' : ''}`}>⌄</span>
          </button>
          {moreOpen && (
            <div className="mt-2">
              <MoreMenuItems
                isOwner={business?.role === 'owner'}
                leaseStatus={leaseStatus}
                userIdentifier={user?.email || user?.phone || ''}
                onLogout={() => void handleLogout()}
              />
            </div>
          )}
        </div>
      </div>

      {moreMenuPopupOpen && (
        <div
          className="fixed inset-0 bg-jb-ink/40 flex items-end sm:items-center justify-center z-50 px-0 sm:px-5"
          onClick={() => setMoreMenuPopupOpen(false)}
        >
          <div
            className="w-full sm:max-w-md max-h-[80vh] overflow-y-auto bg-jb-cream rounded-t-2xl sm:rounded-2xl p-5"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-4">
              <h2 className="text-lg font-medium">More</h2>
              <button
                onClick={() => setMoreMenuPopupOpen(false)}
                aria-label="Close"
                className="text-jb-ink/40 hover:text-jb-ink text-lg leading-none"
              >
                ✕
              </button>
            </div>
            <MoreMenuItems
              isOwner={business?.role === 'owner'}
              leaseStatus={leaseStatus}
              userIdentifier={user?.email || user?.phone || ''}
              onNavigate={() => setMoreMenuPopupOpen(false)}
              onLogout={() => {
                setMoreMenuPopupOpen(false);
                void handleLogout();
              }}
            />
          </div>
        </div>
      )}

      <AppBottomNav />
      {showTour && <DashboardTour onDone={() => setShowTour(false)} />}
    </div>
  );
}
