import type { PageServerLoad } from './$types';
import { schedule } from '$lib/server/api';

/**
 * The weekly programme, from the team's own `schedule_slots`.
 *
 * This used to build a week out of MAL's `broadcast_day`, falling back to
 * `index % 7` when that was missing — which meant the page confidently showed a
 * schedule that was partly invented. Both are gone: the catalog is curated by
 * hand now, so when a Romanian episode lands is a decision someone makes in
 * /admin/program, not something derivable from a Japanese air date.
 *
 * Two weeks rather than one, because a programme is forward-looking and the
 * point of publishing it is to show what is coming.
 */
const DAYS = 14;

export const load: PageServerLoad = async ({ fetch }) => {
  const slots = await schedule(fetch, DAYS);

  // Group into calendar days in the *server's* zone only to establish the
  // column boundaries; each slot keeps its raw instant so the browser can
  // render the time in the viewer's own zone.
  const today = new Date();
  const start = new Date(today.getFullYear(), today.getMonth(), today.getDate());

  const days = Array.from({ length: DAYS }, (_, i) => {
    const d = new Date(start);
    d.setDate(start.getDate() + i);
    const next = new Date(d);
    next.setDate(d.getDate() + 1);
    return {
      iso: d.toISOString(),
      today: i === 0,
      items: slots.filter((s) => {
        const at = new Date(s.scheduledAt);
        return at >= d && at < next;
      })
    };
  });

  return { days, total: slots.length };
};
