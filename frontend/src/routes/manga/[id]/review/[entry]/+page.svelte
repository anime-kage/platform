<script lang="ts">
  import RichText from '$lib/components/RichText.svelte';
  import { mediaUrl } from '$lib/media';
  import CommentSection from '$lib/components/CommentSection.svelte';
  import api from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import { displayName, titleRef } from '$lib/types';

  let { data } = $props();
  const m = $derived(data.manga);
  const r = $derived(data.review);

  const starsOn = $derived(r.score ? Math.round(r.score / 2) : 0);
  const dateLabel = $derived(
    new Date(r.updatedAt).toLocaleDateString('ro-RO', { day: 'numeric', month: 'long', year: 'numeric' })
  );
</script>

<svelte:head>
  <title>Recenzie de {r.user.username} · {displayName(m)} · Anime-Kage</title>
</svelte:head>

<div class="container page">
  <a class="crumb" href={`/manga/${titleRef(m)}`}>← {displayName(m)}</a>

  <div class="layout">
    <aside class="side">
      <a class="poster media-tone" href={`/manga/${titleRef(m)}`}>
        {#if m.imageUrl}<img src={mediaUrl(m.imageUrl)} alt={displayName(m)} width="200" height="300" />{/if}
      </a>
    </aside>

    <main class="main">
      <header class="head">
        <div class="byline">
          <a class="avatar" class:monogram={!r.user.avatarUrl} style={`--mg-hue:${nameHue(r.user.username)}`} href={`/user/${r.user.username}`}>
            {#if r.user.avatarUrl}
              <img src={api.resolveUrl(r.user.avatarUrl)} alt={r.user.username} />
            {:else}
              <span>{r.user.username.charAt(0).toUpperCase()}</span>
            {/if}
          </a>
          <div class="byline-main">
            <p class="by-kicker">Recenzie de <a class="plink" href={`/user/${r.user.username}`}><strong>{r.user.username}</strong></a></p>
            <h1 class="title">
              <a class="title-link" href={`/manga/${titleRef(m)}`}>{displayName(m)}</a>
              {#if m.year}<span class="year">{m.year}</span>{/if}
            </h1>
            <div class="meta">
              {#if starsOn}
                <span class="stars">{'★'.repeat(starsOn)}<span class="stars-off">{'★'.repeat(5 - starsOn)}</span></span>
              {/if}
              <span class="date">{dateLabel}</span>
            </div>
          </div>
        </div>
      </header>

      <article class="body">
        <p class="text"><RichText text={r.notes} /></p>
      </article>

      <div class="comments" id="comentarii">
        <CommentSection mangaId={m.id} reviewId={r.entryId} flat heading="Comentarii" />
      </div>
    </main>
  </div>
</div>

<style>
  .page { padding-block: var(--space-5) var(--space-8); max-width: 1040px; }

  .crumb {
    display: inline-block; margin-bottom: var(--space-5);
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
  }
  .crumb:hover { color: var(--text-primary); }

  .layout { display: grid; grid-template-columns: 200px 1fr; gap: var(--space-6); align-items: start; }

  .poster { display: block; }
  .poster img {
    width: 100%; aspect-ratio: 2/3; object-fit: cover;
    border-radius: var(--radius-lg); border: 1px solid var(--border-default);
    box-shadow: 0 16px 40px rgba(0, 0, 0, 0.45);
  }

  .byline { display: flex; gap: 14px; align-items: flex-start; }
  .avatar {
    flex-shrink: 0; width: 44px; height: 44px; border-radius: 50%; overflow: hidden;
    background: var(--surface-overlay); border: 1px solid var(--border-subtle);
    display: flex; align-items: center; justify-content: center;
    font-family: var(--font-mono); font-size: var(--fs-small); font-weight: 600; color: var(--text-muted);
  }
  .avatar:hover { color: #fff; }
  .avatar img { width: 100%; height: 100%; object-fit: cover; }
  .byline-main { min-width: 0; }

  .by-kicker {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
  }
  .by-kicker strong { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .plink { color: inherit; }
  .plink:hover { color: var(--accent); }

  .title { font-size: clamp(1.5rem, 1.2rem + 1.2vw, 2rem); line-height: 1.1; letter-spacing: -0.015em; margin-top: 6px; }
  .title-link { color: var(--text-primary); }
  .title-link:hover { color: var(--accent); }
  .year { font-family: var(--font-mono); font-size: var(--fs-small); font-weight: 400; color: var(--text-muted); margin-left: 8px; }

  .meta { display: flex; align-items: baseline; gap: 14px; margin-top: 8px; }
  .stars { font-size: 1rem; color: var(--accent); letter-spacing: 2px; }
  .stars-off { color: var(--border-default); }
  .date { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  .body {
    margin-top: var(--space-5); padding-bottom: var(--space-6);
    border-bottom: 1px solid var(--border-subtle);
  }
  .text {
    font-size: 1.125rem; line-height: 1.75; color: var(--text-primary);
    white-space: pre-wrap; word-break: break-word; max-width: 68ch;
  }

  .comments { margin-top: var(--space-6); max-width: 780px; }

  @media (max-width: 700px) {
    .layout { grid-template-columns: minmax(0, 1fr); }
    .side { display: flex; }
    .poster { width: 130px; }
  }
</style>
