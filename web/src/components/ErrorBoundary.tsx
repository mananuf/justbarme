import { Component, type ErrorInfo, type ReactNode } from 'react';

interface Props {
  children: ReactNode;
}

interface State {
  failed: boolean;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Application render failed', error, info.componentStack);
  }

  render() {
    if (this.state.failed) {
      return (
        <main className="min-h-screen bg-jb-cream text-jb-ink flex flex-col items-center justify-center px-6 text-center gap-4">
          <p className="text-xs tracking-widest text-jb-ink/40 uppercase">Something went wrong</p>
          <h1 className="text-2xl font-light tracking-tight">The app could not finish loading.</h1>
          <p className="text-sm text-jb-ink/55 max-w-sm">
            Your locally saved work has not been cleared. Reload to try again.
          </p>
          <button
            type="button"
            onClick={() => window.location.reload()}
            className="px-6 py-3 rounded-xl bg-jb-ink text-jb-cream text-sm font-medium hover:bg-jb-green transition-colors"
          >
            Reload justbarme
          </button>
        </main>
      );
    }
    return this.props.children;
  }
}
