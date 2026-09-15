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
        <main className="fatal-error">
          <p className="eyebrow">Something went wrong</p>
          <h1>The app could not finish loading.</h1>
          <p>Your locally saved work has not been cleared. Reload to try again.</p>
          <button type="button" onClick={() => window.location.reload()}>
            Reload justbarme
          </button>
        </main>
      );
    }
    return this.props.children;
  }
}
