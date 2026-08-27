<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { displayName, titleRef, type Anime, type Manga } from '$lib/types';

  // `ribbon` (e.g. "Ep 8", "Cap 12") marks a release; when set it replaces the
  // type chip in the top-left with an accent pill.
  //
  // `showScore` is the catalog (MAL) score in the corner. Turn it off where
  // the card already carries a rating of its own — two different numbers on
  // one poster, on two different scales, is worse than one.
  let {
    a,
    href,
    ribbon,
    showScore = true
  }: { a: Anime | Manga; href?: string; ribbon?: string; showScore?: boolean } = $props();

  // titleRef, not a.id: the slug when the title has one, so the card links
  // straight to the canonical URL instead of bouncing through a redirect.
  const target = $derived(href ?? `/anime/${titleRef(a)}`);
  const count = $derived(
    'episodes' in a && a.episodes
      ? `${a.episodes} ep`
      : 'chapters' in a && a.chapters
        ? `${a.chapters} cap`
        : null
  );
  const meta = $derived([a.year, count].filter(Boolean).join(' · '));
</script>

<a class="card" href={target}>
  {#if a.imageUrl}
    <img class="art" loading="lazy" src={mediaUrl(a.imageUrl)} alt={displayName(a)} width="240" height="360" />
    <span class="tone" aria-hidden="true"></span>
  {:else}
    <div class="art ph">
      <span class="mark" aria-hidden="true">{displayName(a).charAt(0)}</span>
    </div>
  {/if}

  {#if ribbon}
    <span class="ribbon">{ribbon}</span>
  {:else}
    <span class="type">{a.type ?? 'TV'}</span>
  {/if}
  {#if a.score && showScore}
    <span class="score"><span class="star">★</span>{a.score.toFixed(2)}</span>
  {/if}

  <span class="veil">
    <span class="t">{displayName(a)}</span>
    {#if meta}<span class="m">{meta}</span>{/if}
  </span>
</a>

<style>
  .card {
    position: relative; display: block; aspect-ratio: 2 / 3; overflow: hidden;
    border-radius: var(--radius-md); border: 1px solid var(--border-subtle);
    background: var(--surface-overlay); box-shadow: var(--shadow-1);
    transition:
      transform 0.28s cubic-bezier(0.2, 0.7, 0.2, 1),
      border-color 0.28s var(--ease),
      box-shadow 0.28s var(--ease);
  }
  .card:hover {
    transform: translateY(-5px);
    border-color: color-mix(in srgb, var(--accent) 55%, var(--border-subtle));
    box-shadow: var(--shadow-2);
  }

  /* Real posters are bright/saturated; mute them toward the sketch's
     duotone look, then let hover bring the color back. */
  .art {
    width: 100%; height: 100%; object-fit: cover;
    filter: saturate(0.78) contrast(1.04) brightness(0.9);
    transition: filter 0.28s var(--ease);
  }
  .card:hover .art { filter: saturate(0.95) contrast(1.03) brightness(0.97); }

  /* cool wash that ties every cover into the site palette */
  .tone {
    position: absolute; inset: 0; pointer-events: none;
    background:
      linear-gradient(158deg, color-mix(in srgb, var(--blue) 16%, transparent), transparent 55%),
      radial-gradient(135% 92% at 80% 6%, color-mix(in srgb, var(--cyan) 10%, transparent), transparent 55%);
    mix-blend-mode: soft-light;
  }

  .ph {
    display: grid; place-items: center;
    background:
      linear-gradient(158deg, oklch(0.4 0.055 230) 0%, oklch(0.26 0.045 230) 46%, oklch(0.15 0.028 230) 100%),
      radial-gradient(135% 92% at 80% 6%, oklch(0.55 0.08 230 / 0.45), transparent 55%);
  }
  .mark {
    font-family: var(--font-display); font-weight: var(--fw-semibold);
    font-size: 5.25rem; color: rgba(255, 255, 255, 0.07); user-select: none;
  }

  .type {
    position: absolute; top: 9px; left: 9px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase;
    padding: 3px 7px; border-radius: 5px;
    background: rgba(10, 10, 12, 0.5); color: rgba(255, 255, 255, 0.92);
    backdrop-filter: blur(4px);
  }
  .score {
    position: absolute; top: 9px; right: 9px;
    display: flex; align-items: center; gap: 3px;
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-medium);
    padding: 3px 7px; border-radius: 5px;
    background: rgba(10, 10, 12, 0.5); color: var(--accent);
    backdrop-filter: blur(4px);
  }
  .star { font-size: 0.5625rem; }

  .ribbon {
    position: absolute; top: 9px; left: 9px;
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-bold);
    letter-spacing: 0.04em; padding: 3px 8px; border-radius: 5px;
    background: var(--accent); color: var(--on-accent);
  }

  .veil {
    position: absolute; left: 0; right: 0; bottom: 0;
    display: block; padding: 34px 12px 13px;
    background: linear-gradient(to top, rgba(8, 8, 10, 0.94), rgba(8, 8, 10, 0.5) 56%, transparent);
  }
  .t {
    display: block;
    font-family: var(--font-display); font-size: 1rem; font-weight: var(--fw-semibold);
    line-height: 1.16; color: #fff; text-wrap: pretty;
  }
  .m {
    display: block; margin-top: 5px;
    font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.02em;
    color: rgba(255, 255, 255, 0.6);
  }
</style>
