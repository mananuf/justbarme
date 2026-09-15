import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';

import { apiRequest, ApiError } from '../api/client';
import { Logo } from '../components/Logo';
import { db } from '../lib/db';
import { DeviceCryptoUnsupportedError, getOrCreateDeviceRecord, saveEnrollment } from '../lib/device';
import { checkLease } from '../lib/lease';
import { useSession } from '../lib/session';

type Platform = 'android' | 'ios';

function detectPlatform(): Platform {
  const ua = window.navigator.userAgent || '';
  return /iPhone|iPad|iPod/i.test(ua) ? 'ios' : 'android';
}

const ANDROID_STEPS = [
  { title: 'Tap the browser menu', desc: 'Look for the ⋮ icon in the top-right corner of Chrome.' },
  { title: 'Select “Install app”', desc: 'It may also say “Add to Home screen.”' },
  { title: 'Confirm installation', desc: 'Tap Install when Chrome asks you to confirm.' },
  { title: 'Open justbarme', desc: 'Find it on your home screen, just like any other app.' },
];

const IOS_STEPS = [
  {
    title: 'Tap the Share button',
    desc: "It's the square with an arrow, at the bottom of Safari.",
  },
  {
    title: 'Select “Add to Home Screen”',
    desc: "Scroll down the share sheet if you don't see it right away.",
  },
  { title: 'Tap Add', desc: 'Confirm in the top-right corner.' },
  { title: 'Open justbarme', desc: 'Find it on your home screen, just like any other app.' },
];

function PlatformIcon({ platform }: { platform: Platform }) {
  return (
    <div className="w-10 h-10 rounded-xl border border-jb-ink/10 bg-white flex items-center justify-center">
      {platform === 'android' ? (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#16201A" strokeWidth="1.6">
          <rect x="6" y="2" width="12" height="20" rx="2" />
          <circle cx="12" cy="18" r="0.8" fill="#16201A" />
          <path d="M9 2h6" />
        </svg>
      ) : (
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#16201A" strokeWidth="1.6">
          <rect x="7" y="2" width="10" height="20" rx="2.5" />
          <circle cx="12" cy="18" r="0.8" fill="#16201A" />
        </svg>
      )}
    </div>
  );
}

interface EnrollResponse {
  device: { id: string; display_name: string; status: string; enrolled_at: string };
  lease: string;
}

type EnrollStatus =
  | { kind: 'checking' }
  | { kind: 'no-business' }
  | { kind: 'enrolling' }
  | { kind: 'enrolled'; expiresAt: string }
  | { kind: 'error'; message: string };

function OfflineAccessCard({ status }: { status: EnrollStatus }) {
  if (status.kind === 'checking' || status.kind === 'enrolling') {
    return (
      <div className="mb-6 rounded-xl border border-jb-ink/10 bg-white/70 px-4 py-3 flex items-center gap-3">
        <span className="w-1.5 h-1.5 rounded-full bg-jb-gold animate-pulse" aria-hidden="true" />
        <p className="text-[12.5px] text-jb-ink/60">Setting up offline access for this device…</p>
      </div>
    );
  }
  if (status.kind === 'enrolled') {
    const until = new Date(status.expiresAt).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
    return (
      <div className="mb-6 rounded-xl border border-emerald-600/20 bg-emerald-50 px-4 py-3 flex items-center gap-3">
        <span className="w-1.5 h-1.5 rounded-full bg-emerald-600" aria-hidden="true" />
        <p className="text-[12.5px] text-emerald-800">This device can work offline until {until}.</p>
      </div>
    );
  }
  if (status.kind === 'no-business') {
    return (
      <div className="mb-6 rounded-xl border border-jb-ink/10 bg-white/70 px-4 py-3">
        <p className="text-[12.5px] text-jb-ink/60">Create a business first to enable offline access.</p>
      </div>
    );
  }
  return (
    <div role="alert" className="mb-6 rounded-xl border border-jb-gold/30 bg-jb-gold/10 px-4 py-3">
      <p className="text-[12.5px] text-[#7a5b1f]">{status.message} You can still use justbarme while online.</p>
    </div>
  );
}

export function Install() {
  const [platform, setPlatform] = useState<Platform>(detectPlatform);
  const [enrollStatus, setEnrollStatus] = useState<EnrollStatus>({ kind: 'checking' });
  const { csrfToken, selectedBusinessId } = useSession();

  useEffect(() => {
    let cancelled = false;

    async function enroll() {
      const existing = await db.device.get('current');
      if (existing?.lease) {
        const result = await checkLease(existing.lease, import.meta.env.VITE_OFFLINE_SIGNING_PUBLIC_KEY ?? '');
        if (result.payload && !result.expired) {
          if (!cancelled) setEnrollStatus({ kind: 'enrolled', expiresAt: result.payload.expires_at });
          return;
        }
      }

      if (!selectedBusinessId) {
        if (!cancelled) setEnrollStatus({ kind: 'no-business' });
        return;
      }

      if (!cancelled) setEnrollStatus({ kind: 'enrolling' });
      try {
        const device = await getOrCreateDeviceRecord();
        const response = await apiRequest<EnrollResponse>('/api/v1/devices/enroll', {
          method: 'POST',
          headers: {
            ...(csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
            'X-Business-ID': selectedBusinessId,
          },
          body: JSON.stringify({ public_key: device.publicKeyB64, display_name: device.displayName }),
        });
        await saveEnrollment(response.device.id, response.lease);
        const result = await checkLease(response.lease, import.meta.env.VITE_OFFLINE_SIGNING_PUBLIC_KEY ?? '');
        if (!cancelled) {
          setEnrollStatus({ kind: 'enrolled', expiresAt: result.payload?.expires_at ?? response.device.enrolled_at });
        }
      } catch (err) {
        if (cancelled) return;
        if (err instanceof DeviceCryptoUnsupportedError) {
          setEnrollStatus({ kind: 'error', message: err.message });
        } else if (err instanceof ApiError && err.code === 'CONFLICT') {
          setEnrollStatus({ kind: 'error', message: 'This device is already enrolled elsewhere.' });
        } else {
          setEnrollStatus({ kind: 'error', message: 'Could not set up offline access right now.' });
        }
      }
    }

    void enroll();
    return () => {
      cancelled = true;
    };
  }, [csrfToken, selectedBusinessId]);

  const steps = platform === 'android' ? ANDROID_STEPS : IOS_STEPS;

  return (
    <div className="min-h-screen bg-jb-cream text-jb-ink flex flex-col">
      <div className="w-full max-w-sm mx-auto px-6 pt-8 pb-16 flex-1 flex flex-col">
        <div className="flex items-center gap-4 mb-10">
          <Link
            to="/dashboard"
            aria-label="Back"
            className="text-jb-ink/40 hover:text-jb-ink transition-colors text-lg leading-none"
          >
            ←
          </Link>
          <span className="font-pixel text-[10px] tracking-widest text-jb-ink/35">SETTINGS · INSTALL</span>
        </div>

        <div className="w-16 h-16 rounded-2xl bg-jb-green-dark flex items-center justify-center mb-5">
          <Logo className="w-9 h-9 text-jb-cream" />
        </div>

        <h1 className="text-2xl font-light tracking-tight mb-1.5" style={{ fontFamily: 'var(--font-display)' }}>
          Install justbarme on your phone
        </h1>
        <p className="text-[13px] text-jb-ink/45 mb-6 leading-relaxed">
          Add justbarme to your home screen so you can open it like a normal app — no browser bar, no
          typing a web address. This is the icon you&apos;ll see.
        </p>

        <OfflineAccessCard status={enrollStatus} />

        <div className="flex gap-2 mb-6 p-1 rounded-xl bg-jb-ink/[0.05]">
          {(['android', 'ios'] as Platform[]).map((p) => (
            <button
              key={p}
              onClick={() => setPlatform(p)}
              className={`flex-1 rounded-lg py-2.5 text-[13px] font-medium transition-colors ${
                platform === p ? 'bg-white text-jb-ink shadow-sm' : 'text-jb-ink/45'
              }`}
            >
              {p === 'android' ? 'Android / Chrome' : 'iPhone / Safari'}
            </button>
          ))}
        </div>

        <div className="space-y-3 flex-1">
          {steps.map((step, i) => (
            <div key={step.title} className="flex items-start gap-3.5 rounded-xl border border-jb-ink/10 bg-white p-4">
              <div className="w-7 h-7 rounded-full bg-jb-ink text-jb-cream flex items-center justify-center text-[12px] font-medium shrink-0">
                {i + 1}
              </div>
              <div>
                <h3 className="text-[14px] font-medium text-jb-ink mb-0.5">{step.title}</h3>
                <p className="text-[12.5px] text-jb-ink/45 leading-relaxed">{step.desc}</p>
              </div>
            </div>
          ))}
        </div>

        <div className="flex items-center gap-2.5 mt-6 mb-2">
          <PlatformIcon platform={platform} />
          <p className="text-[11.5px] text-jb-ink/35 leading-relaxed">
            Three simple steps to keep justbarme on your phone — no app store required.
          </p>
        </div>

        <div className="mt-6 flex flex-col gap-2.5">
          <Link
            to="/dashboard"
            className="w-full text-center rounded-xl bg-jb-ink text-jb-cream text-[15px] font-medium py-4 hover:bg-jb-green transition-colors"
          >
            Done — Open justbarme
          </Link>
          <Link to="/dashboard" className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 text-center transition-colors">
            Maybe later
          </Link>
        </div>
        <p className="text-center text-[11px] text-jb-ink/30 mt-2">You can always find this guide in Settings.</p>
      </div>
    </div>
  );
}
