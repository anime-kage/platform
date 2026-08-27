import { writable } from 'svelte/store';
import { browser } from '$app/environment';

export type Theme = 'dark' | 'light' | 'sakura';

/** The cycle the toggle button walks, in order. */
export const THEMES: Theme[] = ['dark', 'light', 'sakura'];

/** Dark is the default and carries no attribute, so the CSS stays the base. */
const attrFor = (t: Theme) => (t === 'dark' ? null : t);

const read = (): Theme => {
  if (!browser) return 'dark';
  const a = document.documentElement.getAttribute('data-theme');
  return a === 'light' || a === 'sakura' ? a : 'dark';
};

// app.html applies the saved theme before first paint; this store just
// mirrors it for the UI and handles cycling.
export const theme = writable<Theme>(read());

export function setTheme(next: Theme) {
  if (browser) {
    const attr = attrFor(next);
    if (attr) document.documentElement.setAttribute('data-theme', attr);
    else document.documentElement.removeAttribute('data-theme');
    try {
      localStorage.setItem('ak-theme', next);
    } catch {
      /* private mode */
    }
  }
  theme.set(next);
}

/** Cycles dark → light → sakura → dark. */
export function toggleTheme() {
  theme.update((t) => {
    const next = THEMES[(THEMES.indexOf(t) + 1) % THEMES.length];
    if (browser) {
      const attr = attrFor(next);
      if (attr) document.documentElement.setAttribute('data-theme', attr);
      else document.documentElement.removeAttribute('data-theme');
      try {
        localStorage.setItem('ak-theme', next);
      } catch {
        /* private mode */
      }
    }
    return next;
  });
}
