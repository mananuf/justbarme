/// <reference types="vite/client" />

interface ImportMetaEnv {
  readonly VITE_OFFLINE_SIGNING_PUBLIC_KEY?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
