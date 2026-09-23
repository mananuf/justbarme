import { useEffect, useState } from 'react';

import { TOUR_STEPS, markDashboardTourSeen } from '../lib/dashboardTour';

// A short, self-guided tour of the Dashboard shown once, automatically, the
// first time a new owner reaches it -- the "walkthrough" onboarding leads
// into, so a brand-new signup never lands on a screen full of buttons with
// no idea what they do. Deliberately not a real product-tour library: this
// targets four fixed, already-stable anchors (#quick-sell, #stock, #bills,
// #more -- the same IDs AppBottomNav's own links jump to), so a small
// hand-rolled spotlight is simpler and lighter than pulling in a dependency
// built for arbitrary, dynamic tours.
export function DashboardTour({ onDone }: { onDone: () => void }) {
  const [stepIndex, setStepIndex] = useState(0);
  const [rect, setRect] = useState<DOMRect | null>(null);
  const step = TOUR_STEPS[stepIndex]!;

  useEffect(() => {
    const frame = requestAnimationFrame(() => {
      const target = document.querySelector(step.target);
      if (!target) {
        // The anchor isn't on screen (e.g. a narrower Dashboard variant) --
        // skip straight past this step rather than spotlighting nothing.
        setRect(null);
        setStepIndex((i) => (i < TOUR_STEPS.length - 1 ? i + 1 : i));
        return;
      }
      target.scrollIntoView({ block: 'center', behavior: 'instant' });
      setRect(target.getBoundingClientRect());
    });
    return () => cancelAnimationFrame(frame);
  }, [step.target]);

  function finish() {
    markDashboardTourSeen();
    onDone();
  }

  function next() {
    if (stepIndex < TOUR_STEPS.length - 1) setStepIndex((i) => i + 1);
    else finish();
  }

  function back() {
    setStepIndex((i) => Math.max(0, i - 1));
  }

  return (
    <div
      className="fixed inset-0 z-50"
      role="dialog"
      aria-modal="true"
      aria-label="Dashboard walkthrough"
    >
      {rect && (
        <div
          className="fixed rounded-2xl pointer-events-none transition-all duration-200"
          style={{
            top: rect.top - 6,
            left: rect.left - 6,
            width: rect.width + 12,
            height: rect.height + 12,
            boxShadow: '0 0 0 9999px rgba(18,26,20,0.72)',
          }}
        />
      )}
      {!rect && <div className="fixed inset-0 bg-[rgba(18,26,20,0.72)]" />}

      <div className="fixed inset-x-0 bottom-0 px-5 pb-[calc(env(safe-area-inset-bottom)+1.25rem)] pt-5">
        <div className="max-w-md mx-auto rounded-2xl bg-jb-cream p-5 shadow-[0_-8px_30px_rgba(0,0,0,0.25)]">
          <div className="flex items-center gap-1.5 mb-3">
            {TOUR_STEPS.map((s, i) => (
              <span
                key={s.target}
                className={`h-1 flex-1 rounded-full ${i <= stepIndex ? 'bg-jb-ink' : 'bg-jb-ink/15'}`}
              />
            ))}
          </div>
          <h2 className="text-[16px] font-medium text-jb-ink mb-1">{step.title}</h2>
          <p className="text-[13px] text-jb-ink/60 leading-relaxed mb-4">{step.body}</p>
          <div className="flex items-center justify-between">
            <button
              onClick={finish}
              className="text-[13px] text-jb-ink/40 hover:text-jb-ink py-2 transition-colors"
            >
              Skip tour
            </button>
            <div className="flex items-center gap-2">
              {stepIndex > 0 && (
                <button
                  onClick={back}
                  className="text-[13px] text-jb-ink/60 hover:text-jb-ink px-4 py-2.5 rounded-xl transition-colors"
                >
                  Back
                </button>
              )}
              <button
                onClick={next}
                className="text-[13px] font-medium bg-jb-ink text-jb-cream px-5 py-2.5 rounded-xl hover:bg-jb-green transition-colors active:scale-[0.98]"
              >
                {stepIndex === TOUR_STEPS.length - 1 ? 'Got it' : 'Next'}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
