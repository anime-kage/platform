import { writable } from 'svelte/store';

export type ToastType = 'success' | 'error' | 'info' | 'warning';

export interface Toast {
  id: string;
  type: ToastType;
  message: string;
  duration?: number;
}

function createToastStore() {
  const { subscribe, update } = writable<Toast[]>([]);

  function addToast(type: ToastType, message: string, duration = 4000) {
    const id = Math.random().toString(36).substring(2, 9);
    const toast: Toast = { id, type, message, duration };

    update(toasts => [...toasts, toast]);

    if (duration > 0) {
      setTimeout(() => {
        removeToast(id);
      }, duration);
    }

    return id;
  }

  function removeToast(id: string) {
    update(toasts => toasts.filter(t => t.id !== id));
  }

  return {
    subscribe,
    /**
     * Suppressed on purpose (August 2026).
     *
     * A toast should tell you something you would not otherwise know. A
     * success toast almost never does: the row appears, the button flips, the
     * page navigates — the interface has already said it. Confirming every
     * add, save and delete meant a popup for actions the member just took
     * deliberately, which is noise, and noise is what makes people stop
     * reading the errors that matter.
     *
     * Kept as a no-op rather than deleted from ~100 call sites: those calls
     * still document intent at the point of the action, and re-enabling is one
     * line here. `error` and `info` are untouched — `info` carries the
     * "Autentifică-te ca să…" prompts, which explain why something did NOT
     * happen and are the opposite of noise.
     */
    success: (_message: string, _duration?: number) => '',
    error: (message: string, duration?: number) => addToast('error', message, duration),
    info: (message: string, duration?: number) => addToast('info', message, duration),
    warning: (message: string, duration?: number) => addToast('warning', message, duration),
    remove: removeToast,
    clear: () => update(() => [])
  };
}

export const toast = createToastStore();
