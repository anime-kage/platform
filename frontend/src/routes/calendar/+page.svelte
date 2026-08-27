<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { displayName } from '$lib/types';

  let { data } = $props();

  /* Day headings and times are formatted in the browser: a slot is an instant,
     and only the viewer knows which day and hour that lands on for them. */
  const DAYS_RO = ['Duminică', 'Luni', 'Marți', 'Miercuri', 'Joi', 'Vineri', 'Sâmbătă'];
  const MONTHS_RO = ['ian', 'feb', 'mar', 'apr', 'mai', 'iun', 'iul', 'aug', 'sep', 'oct', 'nov', 'dec'];

  const dayName = (iso: string) => DAYS_RO[new Date(iso).getDay()];
  const dayDate = (iso: string) => {
    const d = new Date(iso);
    return `${d.getDate()} ${MONTHS_RO[d.getMonth()]}`;
  };
  const hhmm = (iso: string) =>
    new Date(iso).toLocaleTimeString('ro-RO', { hour: '2-digit', minute: '2-digit' });
</script>

<svelte:head><title>Program · Anime-Kage</title></svelte:head>

<div class="container cal">
  <header class="top">
    <div>
      <p class="cal-kicker">Calendar</p>
      <h1>Program săptămânal</h1>
    </div>
    <span class="kicker">
      {data.total === 0
        ? 'nimic programat'
        : `${data.total} ${data.total === 1 ? 'episod' : 'episoade'} programate`}
    </span>
  </header>

  {#if data.total === 0}
    <p class="empty">
      Programul e gol deocamdată. Echipa adaugă aici episoadele odată ce intră la
      tradus, cu ziua și ora la care apar.
    </p>
  {:else}
    <div class="week">
      <!-- Only days that have something: a fortnight of mostly-empty columns
           reads as a broken page, and the programme is deliberately sparse. -->
      {#each data.days.filter((d) => d.items.length) as col (col.iso)}
        <section class="day" class:today={col.today}>
          <h2 class="day-head">
            <span class="day-name">{col.today ? 'Azi' : dayName(col.iso)}</span>
            <span class="day-date">{dayDate(col.iso)}</span>
            {#if col.today}<span class="today-dot"></span>{/if}
          </h2>
          <ul>
            {#each col.items as s (s.id)}
              <li>
                <a
                  class="slot"
                  href={s.published ? `/anime/${s.animeId}/episode/${s.episodeNumber}` : `/anime/${s.animeId}`}
                >
                  <span
                    class="thumb media-tone"
                    style={s.imageUrl ? `background-image:url(${mediaUrl(s.imageUrl)})` : ''}
                  ></span>
                  <span class="main">
                    <span class="t">{displayName(s)}</span>
                    <span class="e">
                      Ep {s.episodeNumber} · {hhmm(s.scheduledAt)}
                      {#if s.published}<span class="live">disponibil</span>{/if}
                    </span>
                    {#if s.note}<span class="note">{s.note}</span>{/if}
                  </span>
                </a>
              </li>
            {/each}
          </ul>
        </section>
      {/each}
    </div>
  {/if}
</div>

<style>
  .cal { padding-block: var(--space-6) var(--space-8); }
  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
    margin-bottom: var(--space-6);
  }
  .cal-kicker { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .top h1 { font-size: clamp(1.8rem, 1.5rem + 1.4vw, 2.375rem); letter-spacing: -0.015em; line-height: 1.05; margin-top: 10px; }

  .week {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
    gap: var(--space-4);
  }
  .day {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); padding: var(--space-4);
  }
  .day.today { border-color: color-mix(in srgb, var(--accent) 45%, transparent); }
  .day-head {
    display: flex; align-items: baseline; gap: 9px;
    padding-bottom: var(--space-3); margin-bottom: var(--space-2);
    border-bottom: 1px solid var(--border-subtle);
  }
  .day-name {
    font-family: var(--font-display); font-size: var(--fs-h3);
    font-weight: var(--fw-semibold); color: var(--text-primary);
  }
  .day.today .day-name { color: var(--accent); }
  .day-date {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }
  .today-dot { width: 7px; height: 7px; border-radius: 50%; background: var(--accent); margin-left: auto; }

  ul { list-style: none; margin: 0; padding: 0; }
  .slot {
    display: flex; align-items: center; gap: 12px;
    padding: 9px 4px; border-bottom: 1px solid var(--border-subtle);
  }
  li:last-child .slot { border-bottom: none; }
  .thumb {
    width: 34px; height: 48px; border-radius: 5px; flex: 0 0 auto;
    background-color: var(--surface-overlay);
    background-size: cover; background-position: center;
  }
  .main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .t {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary);
    line-height: 1.35; text-wrap: pretty;
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2;
    -webkit-box-orient: vertical; overflow: hidden;
  }
  .slot:hover .t { color: var(--accent); }
  .e { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 3px; }
  .live { color: var(--success); margin-left: 5px; }
  .note { display: block; font-size: var(--fs-micro); color: var(--accent); margin-top: 3px; }
  .empty {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-6) var(--space-5); text-align: center; text-wrap: pretty;
    color: var(--text-muted); font-size: var(--fs-small); line-height: 1.6;
    max-width: 52ch; margin: 0 auto;
  }
</style>
