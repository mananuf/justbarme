import { NavLink, Route, Routes } from 'react-router-dom';

import { StatusBadge } from '../components/StatusBadge';
import { useConnectivity } from '../hooks/useConnectivity';
import { useServiceHealth } from '../hooks/useServiceHealth';

const quickActions = [
  { label: 'Sell', marker: '01' },
  { label: 'Stock', marker: '02' },
  { label: 'Expense', marker: '03' },
  { label: 'Tab', marker: '04' },
];

function Home() {
  const isOnline = useConnectivity();
  const serviceHealth = useServiceHealth(isOnline);

  return (
    <div className="app-shell">
      <header className="topbar">
        <a className="brand" href="/" aria-label="justbarme home">
          justbar<span>me</span>
        </a>
        <StatusBadge status={serviceHealth} />
      </header>

      <main>
        <section className="hero" aria-labelledby="welcome-title">
          <div>
            <p className="eyebrow">Your bar, within reach</p>
            <h1 id="welcome-title">
              Run the shift.
              <br />
              Keep the truth.
            </h1>
          </div>
          <p className="hero__copy">
            The foundation is ready. Sales, stock, bills and expenses will stay simple—even when the
            network does not.
          </p>
        </section>

        {serviceHealth === 'unavailable' && (
          <aside className="notice" role="alert">
            <strong>The service cannot be reached.</strong>
            <span>You will still be able to use locally available features.</span>
          </aside>
        )}

        <section className="workspace" aria-labelledby="quick-actions-title">
          <div className="section-heading">
            <div>
              <p className="eyebrow">The Place</p>
              <h2 id="quick-actions-title">Quick actions</h2>
            </div>
            <span>Phase 1 foundation</span>
          </div>

          <div className="quick-grid">
            {quickActions.map((action) => (
              <button className="quick-action" type="button" disabled key={action.label}>
                <span>{action.marker}</span>
                <strong>{action.label}</strong>
                <small>Coming next</small>
              </button>
            ))}
          </div>
        </section>

        <section className="principle-card" aria-labelledby="principle-title">
          <p className="eyebrow">Operating principle</p>
          <h2 id="principle-title">Saved here first. Synced when ready.</h2>
          <p>
            justbarme will confirm work only after it is stored on this device. Connectivity changes
            what is current, never whether a real transaction mattered.
          </p>
        </section>
      </main>

      <nav className="bottom-nav" aria-label="Primary navigation">
        <NavLink to="/" end>
          Home
        </NavLink>
        <span aria-disabled="true">Activity</span>
        <span aria-disabled="true">Settings</span>
      </nav>
    </div>
  );
}

function NotFound() {
  return (
    <main className="fatal-error">
      <p className="eyebrow">404</p>
      <h1>That page is not here.</h1>
      <NavLink to="/">Return home</NavLink>
    </main>
  );
}

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Home />} />
      <Route path="*" element={<NotFound />} />
    </Routes>
  );
}
