import { useEffect, useState } from 'react';

import { updateServiceWorker } from '../pwa';

// pwa.ts's registerSW dispatches "justbarme:update-available" once a new
// version has finished installing in the background and is sitting there
// waiting to activate (registerType: 'prompt' -- deliberately not applied
// automatically, since swapping the app shell out from under someone
// mid-sale could lose in-progress state). Nothing listened for that event
// before this, so an already-open tab -- or a shared receipt link opened
// in a browser that already has the service worker registered from a
// previous visit -- could keep rendering a stale bundle indefinitely after
// every future deploy. This is what actually surfaces the update.
export function UpdateAvailableBanner() {
  const [available, setAvailable] = useState(false);

  useEffect(() => {
    function handleUpdateAvailable() {
      setAvailable(true);
    }
    window.addEventListener('justbarme:update-available', handleUpdateAvailable);
    return () => window.removeEventListener('justbarme:update-available', handleUpdateAvailable);
  }, []);

  if (!available) return null;

  return (
    <div className="fixed inset-x-0 bottom-0 z-[60] px-4 pb-[calc(env(safe-area-inset-bottom)+1rem)]">
      <div className="max-w-md mx-auto rounded-xl bg-jb-ink text-jb-cream px-4 py-3 flex items-center justify-between gap-3 shadow-[0_8px_30px_rgba(0,0,0,0.25)]">
        <span className="text-[13px]">A new version is ready.</span>
        <button
          onClick={() => void updateServiceWorker(true)}
          className="text-[13px] font-medium bg-jb-cream text-jb-ink px-3 py-1.5 rounded-lg hover:bg-white transition-colors shrink-0"
        >
          Refresh
        </button>
      </div>
    </div>
  );
}
