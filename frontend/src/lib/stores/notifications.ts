import { writable } from 'svelte/store';
import { browser } from '$app/environment';
import type { Notification } from '$shared/types';
import { api } from '../api';

interface NotifState {
  items: Notification[];
  unread: number;
  loaded: boolean; // has the full list been fetched at least once
}

const empty: NotifState = { items: [], unread: 0, loaded: false };

// Poll only the cheap unread-count endpoint on a timer; the full list is
// fetched lazily when the user actually opens the bell or the /notificari page.
const POLL_MS = 60_000;

function createNotifStore() {
  const { subscribe, set, update } = writable<NotifState>(empty);
  let timer: ReturnType<typeof setInterval> | null = null;

  async function refreshCount() {
    if (!browser || !api.isAuthenticated()) return;
    try {
      const { unread } = await api.getUnreadCount();
      update((s) => ({ ...s, unread }));
    } catch {
      /* silent — a dropped poll just tries again next tick */
    }
  }

  async function load() {
    if (!browser || !api.isAuthenticated()) return;
    try {
      const { data, unread } = await api.getNotifications();
      set({ items: data, unread, loaded: true });
    } catch {
      /* leave whatever we had */
    }
  }

  async function markAllRead() {
    update((s) => ({ ...s, items: s.items.map((n) => ({ ...n, unread: false })), unread: 0 }));
    try {
      await api.markAllNotificationsRead();
    } catch {
      /* the optimistic clear stands; the next poll reconciles */
    }
  }

  async function markRead(id: number) {
    let wasUnread = false;
    update((s) => {
      const items = s.items.map((n) => {
        if (n.id === id && n.unread) {
          wasUnread = true;
          return { ...n, unread: false };
        }
        return n;
      });
      return { ...s, items, unread: wasUnread ? Math.max(0, s.unread - 1) : s.unread };
    });
    if (!wasUnread) return;
    try {
      await api.markNotificationRead(id);
    } catch {
      /* keep the optimistic state */
    }
  }

  // Begin polling the unread count. Idempotent — safe to call on every login.
  function start() {
    if (!browser || timer) return;
    refreshCount();
    timer = setInterval(refreshCount, POLL_MS);
  }

  function stop() {
    if (timer) {
      clearInterval(timer);
      timer = null;
    }
    set(empty);
  }

  return { subscribe, load, refreshCount, markAllRead, markRead, start, stop };
}

export const notifications = createNotifStore();
