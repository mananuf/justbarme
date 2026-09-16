/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_OFFLINE_SIGNING_PUBLIC_KEY?: string;
  // Google OAuth client ID for "Sign in with Google" (GoogleSignInButton.tsx).
  // A public identifier, not a secret. Unset disables the button entirely --
  // see CLAUDE.md's OAuth section.
  readonly VITE_GOOGLE_OAUTH_CLIENT_ID?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
