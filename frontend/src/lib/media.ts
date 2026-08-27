// Resolve a media path (poster, avatar, page image) to a browser-loadable URL.
//
// Uploaded files come back as backend-relative paths like `/uploads/posters/x.png`.
// Rendered as-is they resolve against the FRONTEND origin (:5173) and 404 — the
// files live on the backend. Jikan/MAL posters are already absolute `https://…`
// URLs and must pass through untouched. VITE_API_URL is the browser-facing
// backend origin (its port is published), so an absolute `${BASE}/uploads/…`
// loads fine both from SSR-rendered HTML and client-side.
//
// This is the single source of truth; `ApiClient.resolveUrl` delegates here.
const BASE = (import.meta.env.VITE_API_URL || 'http://localhost:3000').replace(/\/$/, '');

export function mediaUrl(path?: string | null): string {
  if (!path) return '';
  if (path.startsWith('http://') || path.startsWith('https://') || path.startsWith('data:')) return path;
  return `${BASE}${path}`;
}
