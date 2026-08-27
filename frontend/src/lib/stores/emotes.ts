import { writable, get } from 'svelte/store';
import api from '$lib/api';
import type { Emote } from '$shared/types';

/**
 * The active emote set, fetched once per session.
 *
 * Every chat message and the picker need it, and it changes only when an admin
 * uploads one — so it is loaded on first use and shared, rather than refetched
 * per component. `reload()` is for the admin tab, which has just changed it.
 */
export const emotes = writable<Emote[]>([]);

let loaded = false;
let inflight: Promise<void> | null = null;

export async function loadEmotes(force = false): Promise<void> {
  if (loaded && !force) return;
  if (inflight && !force) return inflight;
  inflight = (async () => {
    try {
      emotes.set((await api.getEmotes()).data);
      loaded = true;
    } catch {
      // An emote that fails to load renders as its plain code, which is a
      // readable message rather than a broken one.
    } finally {
      inflight = null;
    }
  })();
  return inflight;
}

/** code → url, the shape the tokenizer wants. */
export function emoteMap(list?: Emote[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (const e of list ?? get(emotes)) out[e.code] = e.imageUrl;
  return out;
}
