import { useEffect, useRef } from 'react';

import { describeActionError } from '../lib/errors';
import { useSession } from '../lib/session';

const GIS_SCRIPT_SRC = 'https://accounts.google.com/gsi/client';

interface GoogleCredentialResponse {
  credential: string;
}

declare global {
  interface Window {
    google?: {
      accounts: {
        id: {
          initialize: (config: {
            client_id: string;
            callback: (response: GoogleCredentialResponse) => void;
          }) => void;
          renderButton: (parent: HTMLElement, options: Record<string, unknown>) => void;
        };
      };
    };
  }
}

// Loaded lazily, once, and cached on the module -- not every mount of
// GoogleSignInButton (Login and SignUp both render one) should inject a
// second copy of Google's script tag.
let scriptLoadPromise: Promise<void> | null = null;

function loadGoogleIdentityScript(): Promise<void> {
  if (window.google?.accounts?.id) {
    return Promise.resolve();
  }
  scriptLoadPromise ??= new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = GIS_SCRIPT_SRC;
    script.async = true;
    script.defer = true;
    script.onload = () => resolve();
    script.onerror = () => reject(new Error('failed to load Google Identity Services'));
    document.head.appendChild(script);
  });
  return scriptLoadPromise;
}

interface GoogleSignInButtonProps {
  onSuccess: () => void;
  onError: (message: string) => void;
}

// Renders Google's own "Sign in with Google" button via Google Identity
// Services, loaded lazily and only when VITE_GOOGLE_OAUTH_CLIENT_ID is set
// -- renders nothing otherwise, matching the backend's own "unset means
// simply not there" behavior for this feature (see CLAUDE.md's OAuth
// section). Used identically on both Login and SignUp: there is no
// meaningful difference between "sign in" and "sign up" with Google, since
// neither asks for a password up front -- the backend resolves which case
// it is.
export function GoogleSignInButton({ onSuccess, onError }: GoogleSignInButtonProps) {
  const { continueWithGoogle } = useSession();
  const containerRef = useRef<HTMLDivElement | null>(null);
  const clientId = import.meta.env.VITE_GOOGLE_OAUTH_CLIENT_ID;

  useEffect(() => {
    if (!clientId || !containerRef.current) return;
    let cancelled = false;
    const container = containerRef.current;

    void loadGoogleIdentityScript()
      .then(() => {
        if (cancelled || !window.google) return;
        window.google.accounts.id.initialize({
          client_id: clientId,
          callback: (response) => {
            void (async () => {
              try {
                await continueWithGoogle(response.credential);
                onSuccess();
              } catch (err) {
                onError(describeActionError(err, 'Could not reach justbarme.'));
              }
            })();
          },
        });
        window.google.accounts.id.renderButton(container, {
          type: 'standard',
          theme: 'outline',
          size: 'large',
          shape: 'pill',
          width: container.offsetWidth || 320,
        });
      })
      .catch(() => {
        // Script failed to load (offline, an ad/tracker blocker, etc.) --
        // leave the container empty rather than surfacing an error for a
        // sign-in method the person hasn't even tried yet.
      });

    return () => {
      cancelled = true;
    };
  }, [clientId, continueWithGoogle, onSuccess, onError]);

  if (!clientId) {
    return null;
  }

  return <div ref={containerRef} className="flex justify-center" />;
}
