import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { AppBottomNav } from '../components/AppBottomNav';
import { Logo } from '../components/Logo';
import { StatusBadge } from '../components/StatusBadge';
import { useConnectivity } from '../hooks/useConnectivity';
import { useServiceHealth } from '../hooks/useServiceHealth';
import { db } from '../lib/db';
import { checkLease } from '../lib/lease';
import { useSession } from '../lib/session';

type LeaseStatus = { kind: 'none' } | { kind: 'valid'; expiresAt: string } | { kind: 'expired' };

function useLeaseStatus(): LeaseStatus {
  const [status, setStatus] = useState<LeaseStatus>({ kind: 'none' });
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const record = await db.device.get('current');
      if (!record?.lease) return;
      const result = await checkLease(record.lease, import.meta.env.VITE_OFFLINE_SIGNING_PUBLIC_KEY ?? '');
      if (cancelled || !result.payload) return;
      setStatus(result.expired ? { kind: 'expired' } : { kind: 'valid', expiresAt: result.payload.expires_at });
    })();
    return () => {
      cancelled = true;
    };
  }, []);
  return status;
}

export function Dashboard() {
  const [showGuide, setShowGuide] = useState(true);
  const [moreOpen, setMoreOpen] = useState(false);
  const isOnline = useConnectivity();
  const serviceHealth = useServiceHealth(isOnline);
  const { user, memberships, selectedBusinessId, logout } = useSession();
  const navigate = useNavigate();
  const leaseStatus = useLeaseStatus();

  const business = memberships.find((m) => m.businessId === selectedBusinessId) ?? memberships[0];

  async function handleLogout() {
    await logout();
    void navigate('/login', { replace: true });
  }

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink pb-28">
      <div className="max-w-md mx-auto px-5 pt-6">
        <div className="flex items-center justify-between mb-5">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-jb-green-dark flex items-center justify-center shrink-0">
              <Logo className="w-5 h-5 text-jb-cream" />
            </div>
            <div>
              <div className="text-[12px] text-jb-ink/45">{business?.businessName ?? 'justbarme'}</div>
              <h1 className="text-xl font-medium">Good evening{user ? `, ${user.name.split(' ')[0]}` : ''} 👋</h1>
            </div>
          </div>
          <StatusBadge status={serviceHealth} />
        </div>

        {leaseStatus.kind === 'expired' && (
          <div className="mb-5 rounded-xl border border-jb-gold/30 bg-jb-gold/10 px-4 py-3" role="alert">
            <p className="text-[12.5px] text-[#7a5b1f]">
              Offline access on this device has expired. Connect to the internet to renew it.
            </p>
          </div>
        )}

        {showGuide && (
          <div className="mb-5 rounded-xl border border-jb-ink/10 bg-white/70 px-4 py-3 flex items-start justify-between gap-3">
            <p className="text-[12.5px] text-jb-ink/60 leading-relaxed">
              Start by recording your first sale. You can customize your products and settings anytime.
            </p>
            <button onClick={() => setShowGuide(false)} className="text-jb-ink/30 hover:text-jb-ink text-sm shrink-0">
              ✕
            </button>
          </div>
        )}

        <div className="rounded-2xl bg-jb-green-dark text-jb-cream p-5 mb-4">
          <div className="text-[11px] text-jb-cream/60 tracking-wide">TODAY&apos;S SALES</div>
          <div className="text-[34px] font-light mt-1" style={{ fontFamily: 'var(--font-display)' }}>
            ₦48,200
          </div>
          <div className="flex gap-4 mt-3 text-[12px] text-jb-cream/70">
            <span>32 items sold</span>
            <span>·</span>
            <span>₦6,300 expenses</span>
            <span>·</span>
            <span>4 tabs open</span>
          </div>
        </div>

        <Link
          to="/dashboard/sell"
          className="w-full rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 flex items-center justify-center gap-2 active:scale-[0.98] transition-transform mb-4"
        >
          <span className="text-xl leading-none">+</span> Quick Sell
        </Link>

        <div id="stock" className="grid grid-cols-2 gap-3 mb-4 scroll-mt-20">
          <div className="rounded-xl bg-white/70 border border-jb-ink/[0.08] p-4">
            <div className="text-[11px] text-jb-ink/45">Stock alerts</div>
            <div className="text-[15px] font-medium text-jb-ink mt-1">3 items low</div>
            <div className="mt-2 inline-block text-[10px] px-2 py-0.5 rounded-full bg-jb-gold/20 text-[#7a5b1f] font-medium">
              Gordon&apos;s 75cl
            </div>
          </div>
          <div id="bills" className="rounded-xl bg-white/70 border border-jb-ink/[0.08] p-4 scroll-mt-20">
            <div className="text-[11px] text-jb-ink/45">Outstanding</div>
            <div className="text-[15px] font-medium text-jb-ink mt-1">₦11,400</div>
            <div className="mt-2 inline-block text-[10px] px-2 py-0.5 rounded-full bg-jb-ink/[0.06] text-jb-ink/60 font-medium">
              2 open tabs
            </div>
          </div>
        </div>

        <div className="mb-4">
          <div className="text-[11px] text-jb-ink/45 tracking-wide mb-2">RECENT ACTIVITY</div>
          <div className="space-y-2">
            {[
              { who: 'Tolu', what: 'Recorded a sale — Table 4', amt: '₦3,600' },
              { who: 'You', what: 'Received stock — Heineken', amt: '+24' },
              { who: 'Ada', what: 'Recorded payment — Chidi', amt: '₦5,000' },
              { who: 'System', what: 'Flagged a stock count mismatch', amt: 'Review' },
            ].map((row) => (
              <div key={row.what} className="flex items-center justify-between rounded-xl px-3.5 py-3 bg-white/60 border border-jb-ink/[0.05]">
                <div>
                  <div className="text-[13px] text-jb-ink/80">{row.what}</div>
                  <div className="text-[11px] text-jb-ink/40">{row.who}</div>
                </div>
                <div className="text-[12.5px] text-jb-ink/55">{row.amt}</div>
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
            <div className="mt-2 rounded-xl border border-jb-ink/10 bg-white/60 divide-y divide-jb-ink/[0.06] overflow-hidden">
              {[
                { label: 'Expenses', desc: 'Ice, transport, fuel and supplies' },
                { label: 'Reports', desc: 'Sales, stock and expense summaries' },
                { label: 'Activity', desc: 'Everything the team has done today' },
              ].map((row) => (
                <div key={row.label} className="px-4 py-3.5 flex flex-col">
                  <span className="text-[13px] text-jb-ink/75">{row.label}</span>
                  <span className="text-[11px] text-jb-ink/40">{row.desc}</span>
                </div>
              ))}
              <Link to="/install" className="px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors">
                <span className="text-[13px] text-jb-ink/75">Settings — Install justbarme</span>
                <span className="text-[11px] text-jb-ink/40">
                  {leaseStatus.kind === 'valid'
                    ? `Works offline until ${new Date(leaseStatus.expiresAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`
                    : 'Add justbarme to your home screen'}
                </span>
              </Link>
              <button
                onClick={() => void handleLogout()}
                className="w-full text-left px-4 py-3.5 flex flex-col hover:bg-jb-ink/[0.03] transition-colors"
              >
                <span className="text-[13px] text-jb-ink/75">Sign out</span>
                <span className="text-[11px] text-jb-ink/40">{user?.email}</span>
              </button>
            </div>
          )}
        </div>
      </div>

      <AppBottomNav />
    </div>
  );
}
