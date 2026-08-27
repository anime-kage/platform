<script lang="ts">
  import Markdown from '$lib/components/Markdown.svelte';
  import CommentSection from '$lib/components/CommentSection.svelte';

  let { data } = $props();
  const p = $derived(data.post);

  const fullDate = (iso: string) =>
    new Date(iso).toLocaleDateString('ro-RO', { day: 'numeric', month: 'long', year: 'numeric' });
</script>

<svelte:head><title>{p.title} · Știri · Anime-Kage</title></svelte:head>

<article class="container post">
  <a class="back" href="/anunturi">← Toate anunțurile</a>

  <header class="head">
    <p class="meta">
      <span class="tag">{p.tag}</span>
      <span class="sep">·</span>
      <span>{fullDate(p.createdAt)}</span>
      {#if p.authorName}<span class="sep">·</span><span>de {p.authorName}</span>{/if}
      {#if !p.isPublished}<span class="draft">ciornă</span>{/if}
    </p>
    <h1>{p.title}</h1>
  </header>

  {#if p.body}
    <Markdown source={p.body} />
  {/if}

  {#if p.url}
    <p class="cta"><a class="btn fill" href={p.url}>Deschide →</a></p>
  {/if}

  <!-- Same component the episode and review pages use, so replies, voting,
       reporting and moderation behave identically here. -->
  <div class="comments">
    <CommentSection announcementId={p.id} heading="Comentarii" />
  </div>

  {#if data.others.length}
    <section class="more">
      <p class="more-h">Citește și</p>
      {#each data.others as o (o.id)}
        <a class="more-row" href={`/anunturi/${o.slug ?? o.id}`}>
          <span class="more-tag">{o.tag}</span>
          <span class="more-t">{o.title}</span>
        </a>
      {/each}
    </section>
  {/if}
</article>

<style>
  .post { max-width: 760px; padding-block: var(--space-5) var(--space-8); }
  .back { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .back:hover { color: var(--accent); }

  .head { padding: var(--space-5) 0 var(--space-5); border-bottom: 1px solid var(--border-subtle); margin-bottom: var(--space-5); }
  .meta { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .tag { letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent); font-weight: var(--fw-semibold); }
  .sep { color: var(--text-faint); }
  .draft {
    font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.08em;
    padding: 2px 8px; border-radius: var(--radius-pill);
    background: var(--surface-overlay); color: var(--text-muted);
  }
  h1 {
    font-family: var(--font-display); font-size: clamp(1.7rem, 1.4rem + 1.5vw, 2.25rem);
    line-height: 1.15; letter-spacing: -0.015em; margin-top: 12px; text-wrap: pretty;
  }

  .cta { margin-top: var(--space-5); }
  .btn {
    display: inline-block; font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 11px 20px; border-radius: var(--radius-md);
  }
  .btn.fill { background: var(--accent); color: var(--on-accent); }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }

  .comments { margin-top: var(--space-7); }

  .more { margin-top: var(--space-7); border-top: 1px solid var(--border-subtle); padding-top: var(--space-5); }
  .more-h {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted); margin-bottom: 12px;
  }
  .more-row { display: flex; align-items: baseline; gap: 10px; padding: 10px 0; border-bottom: 1px solid var(--border-subtle); }
  .more-row:last-child { border-bottom: none; }
  .more-tag { font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.08em; color: var(--accent); flex: 0 0 auto; }
  .more-t { font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .more-row:hover .more-t { color: var(--accent); }
</style>
