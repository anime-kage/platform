<script lang="ts">
  import RichText from '$lib/components/RichText.svelte';
  import { mediaUrl } from '$lib/media';
  import { authStore } from '$lib/stores/auth';
  import { displayName } from '$lib/types';

  let { data } = $props();
  const auth = $derived($authStore);
  const isSelf = $derived(data.kind === 'real' && auth.user?.username === data.handle);

  const count = $derived(data.kind === 'real' ? data.userReviews.length : data.reviews.length);

  const revTitle = (t: { title: string; titleRomanian?: string }) => t.titleRomanian || t.title;
  const fmtDate = (d: Date | string) =>
    new Date(d).toLocaleDateString('ro-RO', { day: 'numeric', month: 'long', year: 'numeric' });
</script>

<svelte:head><title>Recenzii · {data.name} · Anime-Kage</title></svelte:head>

<div class="container revpage">
  <header class="top">
    <div>
      <p class="crumb"><a href={`/user/${data.handle}`}>← {data.name}</a></p>
      <h1>{isSelf ? 'Recenziile mele' : `Recenziile lui ${data.name}`}</h1>
    </div>
    <span class="count">{count} recenzii</span>
  </header>

  {#if data.kind === 'seed'}
    <div class="revs">
      {#each data.reviews as r, i (i)}
        <article class="rev">
          <a class="rev-thumb media-tone" href={`/anime/${r.anime.id}`} style={r.anime.imageUrl ? `background-image:url(${mediaUrl(r.anime.imageUrl)})` : ''} aria-label={displayName(r.anime)}></a>
          <div class="rev-main">
            <div class="rev-head">
              <a class="rev-t" href={`/anime/${r.anime.id}`}><em>{displayName(r.anime)}</em></a>
              <span class="stars">{'★'.repeat(r.rating)}<span class="stars-off">{'★'.repeat(5 - r.rating)}</span></span>
            </div>
            <p class="rev-text">{r.text}</p>
            <p class="rev-meta">{r.date} · ♥ {r.likes}</p>
          </div>
        </article>
      {/each}
    </div>
  {:else if data.userReviews.length}
    <div class="revs">
      {#each data.userReviews as r (r.kind + r.entryId)}
        <article class="rev">
          <a
            class="rev-thumb media-tone"
            href={`/${r.kind}/${r.title.id}`}
            style={r.title.imageUrl ? `background-image:url(${mediaUrl(r.title.imageUrl)})` : ''}
            aria-label={revTitle(r.title)}
          ></a>
          <div class="rev-main">
            <div class="rev-head">
              <a class="rev-t" href={`/${r.kind}/${r.title.id}`}>
                <em>{revTitle(r.title)}</em>
                {#if r.title.year}<span class="rev-y">{r.title.year}</span>{/if}
              </a>
              {#if r.score}
                <span class="stars">{'★'.repeat(Math.round(r.score / 2))}<span class="stars-off">{'★'.repeat(5 - Math.round(r.score / 2))}</span></span>
              {/if}
            </div>
            <p class="rev-text"><RichText text={r.notes} /></p>
            <p class="rev-meta">
              {r.kind === 'anime' ? 'anime' : 'manga'} · {fmtDate(r.updatedAt)}
              · <a class="rev-go" href={`/${r.kind}/${r.title.id}/review/${r.entryId}`}>💬 {r.replyCount ? `${r.replyCount} comentarii` : 'Comentează'} →</a>
            </p>
          </div>
        </article>
      {/each}
    </div>
  {:else if isSelf}
    <p class="muted">
      Încă nicio recenzie — scrie câteva rânduri la un titlu din
      <a class="inline-link" href="/lista">watchlistul tău</a> și apare aici.
    </p>
  {:else}
    <p class="muted">{data.name} nu a scris încă nicio recenzie.</p>
  {/if}
</div>

<style>
  .revpage { max-width: 760px; padding-block: var(--space-6) var(--space-8); }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
    margin-bottom: var(--space-2);
  }
  .crumb { font-family: var(--font-mono); font-size: var(--fs-caption); }
  .crumb a { color: var(--text-muted); }
  .crumb a:hover { color: var(--text-primary); }
  .top h1 { font-size: clamp(1.5rem, 1.2rem + 1.2vw, 1.9rem); letter-spacing: -0.015em; margin-top: 8px; }
  .count { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .muted { padding-top: var(--space-5); color: var(--text-muted); font-size: var(--fs-small); }
  .inline-link { color: var(--accent); font-weight: var(--fw-semibold); }

  /* flat entries on hairlines */
  .revs { display: flex; flex-direction: column; }
  .rev { display: flex; gap: 18px; padding: 22px 0; border-bottom: 1px solid var(--border-subtle); }
  .rev-thumb {
    width: 58px; height: 86px; border-radius: 6px; flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }
  .rev-main { flex: 1; min-width: 0; }
  .rev-head { display: flex; align-items: baseline; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
  .rev-t em {
    font-family: var(--font-display); font-size: var(--fs-h3);
    font-weight: var(--fw-semibold); font-style: italic; color: var(--text-primary);
  }
  .rev-t:hover em { color: var(--accent); }
  .rev-y { margin-left: 8px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .stars { color: var(--accent); font-size: 0.8125rem; letter-spacing: 1.5px; }
  .stars-off { color: var(--surface-overlay); }
  .rev-text { margin-top: 8px; font-size: var(--fs-small); line-height: 1.65; color: var(--text-muted); }
  .rev-meta { margin-top: 10px; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .rev-go { color: var(--text-muted); }
  .rev-go:hover { color: var(--accent); }
</style>
