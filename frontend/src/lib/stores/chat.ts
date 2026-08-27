import { writable } from 'svelte/store';

// Open/closed state of the docked live-chat drawer. Shared so the layout can
// make room for the panel on the team pages (translate/verify/publish/admin),
// where the right edge is a working column the chat must not cover.
export const chatOpen = writable(false);

/** Restore the saved state (client-only — call from an effect). */
export function initChatOpen() {
  const saved = localStorage.getItem('ak-chat-open');
  chatOpen.set(saved !== null ? saved === '1' : window.matchMedia('(min-width: 1280px)').matches);
}

export function toggleChat() {
  chatOpen.update((v) => {
    localStorage.setItem('ak-chat-open', v ? '0' : '1');
    return !v;
  });
}
