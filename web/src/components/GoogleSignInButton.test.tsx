import { render } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { GoogleSignInButton } from './GoogleSignInButton';
import { SessionProvider } from '../lib/session';

function renderButton() {
  return render(
    <MemoryRouter>
      <SessionProvider>
        <GoogleSignInButton onSuccess={() => undefined} onError={() => undefined} />
      </SessionProvider>
    </MemoryRouter>,
  );
}

describe('GoogleSignInButton', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  it('renders nothing when no Google OAuth client ID is configured', () => {
    vi.stubEnv('VITE_GOOGLE_OAUTH_CLIENT_ID', '');
    const { container } = renderButton();
    expect(container).toBeEmptyDOMElement();
  });

  it('renders a container to hold the Google-drawn button when a client ID is configured', () => {
    vi.stubEnv('VITE_GOOGLE_OAUTH_CLIENT_ID', 'test-client-id.apps.googleusercontent.com');
    const { container } = renderButton();
    expect(container.querySelector('div')).not.toBeNull();
  });
});
