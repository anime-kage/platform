<script lang="ts">
  import GifPicker from '$lib/components/GifPicker.svelte';
  import SpoilerButton from '$lib/components/SpoilerButton.svelte';
  import RichText from '$lib/components/RichText.svelte';
  import { goto, invalidateAll } from '$app/navigation';
  import PosterGrid from '$lib/components/PosterGrid.svelte';
  import CommentSection from '$lib/components/CommentSection.svelte';
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import StatusPicker from '$lib/components/StatusPicker.svelte';
  import { authStore } from '$lib/stores/auth';
  import api from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import { toast } from '$lib/stores/toast';
  import { displayName, displaySynopsis, genreRo, titleRef } from '$lib/types';
  import type { Review } from '$shared/types';

  let { data } = $props();
  const m = $derived(data.manga);
  const auth = $derived($authStore);

  let tab = $state<'capitole' | 'recenzii' | 'comentarii'>('capitole');
  let listBusy = $state(false);
  let inList = $state(false);
  let stars = $state(0); // 1-5 (backend score is 1-10)
  let hoverStar = $state(0);

  type RStatus = 'reading' | 'completed' | 'plan-to-read' | 'on-hold' | 'dropped';
  const R_LABELS: [RStatus, string][] = [
    ['reading', 'În lectură'],
    ['completed', 'Citit'],
    ['plan-to-read', 'În plan'],
    ['on-hold', 'În așteptare'],
    ['dropped', 'Abandonat']
  ];
  // '' = not in list; otherwise the entry's real status — sent back on every
  // upsert so a rating or review never clobbers it
  let rStatus = $state<RStatus | ''>('');

  // reviews
  let reviews = $state<Review[]>([]);
  let reviewsLoaded = $state(false);
  let myReview = $state('');
  let reviewEl = $state<HTMLTextAreaElement | null>(null);
  let savingReview = $state(false);

  // comment scope: 0 = whole series, otherwise a chapter id
  let commentChapterId = $state(0);

  const statusLabel = (s: string | null | undefined) =>
    s === 'publishing' ? 'În publicare'
    : s === 'completed' ? 'Finalizat'
    : s === 'hiatus' ? 'Hiatus'
    : s === 'discontinued' ? 'Întrerupt'
    : s === 'upcoming' ? 'În curând'
    : (s ?? '—');

  // the author renders separately as a link into the filtered library
  const metaLine = $derived(
    [m.year, `${m.chapters ?? '?'} capitole`, m.volumes ? `${m.volumes} volume` : null]
      .filter(Boolean)
      .join(' · ')
  );
  const author = $derived(m.authors?.[0] ?? null);

  const chapNum = (n: string) => String(parseFloat(n));

  // inline catalog edit — same shape as the anime page
  const canEdit = $derived(
    ['admin', 'coordinator', 'translator'].includes(auth.user?.role ?? '')
  );
  const canPoster = $derived(['admin', 'coordinator'].includes(auth.user?.role ?? ''));
  let editOpen = $state(false);
  let editTitleRo = $state('');
  let editSynRo = $state('');
  let editSaving = $state(false);
  let posterBusy = $state(false);

  function openEdit() {
    editTitleRo = m.titleRomanian ?? '';
    editSynRo = m.synopsisRomanian ?? '';
    editOpen = !editOpen;
  }

  async function saveEdit() {
    editSaving = true;
    try {
      await api.patchManga(m.id, {
        ...(editTitleRo.trim() ? { titleRomanian: editTitleRo.trim() } : {}),
        ...(editSynRo.trim() ? { synopsisRomanian: editSynRo.trim() } : {})
      });
      toast.success('Serie actualizată.');
      editOpen = false;
      await invalidateAll();
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Eroare la salvare.');
    } finally {
      editSaving = false;
    }
  }

  async function replacePoster(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    posterBusy = true;
    try {
      await api.uploadPoster('manga', m.id, file);
      toast.success('Poster înlocuit.');
      await invalidateAll();
    } catch (err) {
      toast.error((err as { error?: string }).error ?? 'Eroare la încărcarea posterului.');
    } finally {
      posterBusy = false;
      input.value = '';
    }
  }

  const rateText = $derived(stars ? `Nota ta: ${stars}/5` : 'Evaluează');

  function requireAuth(): boolean {
    if (!auth.isAuthenticated) {
      goto(`/login?redirect=/manga/${m.id}`);
      return false;
    }
    return true;
  }

  // the readlist button is the "de citit" shelf (plan-to-read), separate
  // from the status marker — marking "Citit" etc. must not light it up
  const inReadlist = $derived(rStatus === 'plan-to-read');

  async function toggleList() {
    if (!requireAuth()) return;
    listBusy = true;
    try {
      if (inReadlist) {
        await api.removeFromReadlist(m.id);
        inList = false;
        rStatus = '';
        stars = 0;
        toast.success('Eliminat din readlist.');
      } else {
        if (inList) await api.updateReadlistEntry(m.id, { status: 'plan-to-read' });
        else await api.addToReadlist(m.id, { status: 'plan-to-read' });
        inList = true;
        rStatus = 'plan-to-read';
        toast.success('Adăugat în readlist!');
      }
    } catch {
      toast.error('Eroare la actualizarea readlist-ului.');
    } finally {
      listBusy = false;
    }
  }

  async function setStatus(next: RStatus) {
    if (!requireAuth()) return;
    listBusy = true;
    try {
      const form = {
        status: next,
        ...(next === 'completed' && m.chapters ? { chaptersRead: m.chapters } : {}),
        ...(stars ? { score: stars * 2 } : {})
      };
      if (inList) await api.updateReadlistEntry(m.id, form);
      else await api.addToReadlist(m.id, form);
      inList = true;
      rStatus = next;
      toast.success(`Status: ${R_LABELS.find(([s]) => s === next)?.[1] ?? next}.`);
    } catch {
      toast.error('Eroare la actualizare.');
    } finally {
      listBusy = false;
    }
  }

  async function clearStatus() {
    if (!requireAuth()) return;
    listBusy = true;
    try {
      await api.removeFromReadlist(m.id);
      inList = false;
      rStatus = '';
      stars = 0;
      toast.success('Eliminat din listă.');
    } catch {
      toast.error('Eroare la actualizare.');
    } finally {
      listBusy = false;
    }
  }

  async function rate(n: number) {
    if (!requireAuth()) return;
    const prev = stars;
    stars = n;
    try {
      const form = {
        status: rStatus || ('plan-to-read' as const),
        score: n * 2
      };
      if (inList) await api.updateReadlistEntry(m.id, form);
      else await api.addToReadlist(m.id, form);
      inList = true;
      toast.success(`Notă salvată: ${n}/5.`);
    } catch {
      stars = prev;
      toast.error('Eroare la salvarea notei.');
    }
  }

  async function loadReviews() {
    try {
      const res = await api.getMangaReviews(m.id);
      reviews = res.data;
    } catch {
      reviews = [];
    } finally {
      reviewsLoaded = true;
    }
  }

  async function submitReview() {
    if (!requireAuth()) return;
    const text = myReview.trim();
    if (!text) return;
    savingReview = true;
    try {
      const form = {
        status: rStatus || ('plan-to-read' as const),
        notes: text,
        ...(stars ? { score: stars * 2 } : {})
      };
      if (inList) await api.updateReadlistEntry(m.id, form);
      else await api.addToReadlist(m.id, form);
      inList = true;
      toast.success('Recenzie publicată!');
      await loadReviews();
    } catch {
      toast.error('Eroare la publicarea recenziei.');
    } finally {
      savingReview = false;
    }
  }

  const reviewDate = (d: Date | string) =>
    new Date(d).toLocaleDateString('ro-RO', { day: 'numeric', month: 'short', year: 'numeric' });

  $effect(() => {
    if (tab === 'recenzii' && !reviewsLoaded) loadReviews();
  });

  $effect(() => {
    if (auth.isAuthenticated && m.id) {
      api
        .getReadlistEntry(m.id)
        .then((res) => {
          inList = true;
          rStatus = res.data.status as RStatus;
          stars = res.data.score ? Math.round(res.data.score / 2) : 0;
          if (res.data.notes) myReview = res.data.notes;
        })
        .catch(() => {
          inList = false;
          rStatus = '';
          stars = 0;
        });
    }
  });
</script>

<svelte:head><title>{displayName(m)} · Anime-Kage</title></svelte:head>

<!-- HERO -->
<section class="hero">
  {#if m.imageUrl}
    <div class="hero-bg" style={`background-image:url(${api.resolveUrl(m.imageUrl)})`}></div>
  {/if}
  <div class="hero-fade"></div>
  <div class="container hero-in">
    <a class="crumb" href="/manga">← Bibliotecă</a>
    <div class="hero-grid">
      <div class="poster">
        {#if m.imageUrl}<img src={api.resolveUrl(m.imageUrl)} alt={displayName(m)} width="240" height="360" />{/if}
      </div>

      <div class="head">
        <div class="badges">
          <span class="type-badge">{m.type ?? 'manga'}</span>
          <span class="status" class:live={m.status === 'publishing'}>{statusLabel(m.status)}</span>
          {#if canEdit}
            <button class="edit-btn" class:on={editOpen} onclick={openEdit} title="Editează seria">✎ Editează</button>
          {/if}
        </div>
        <h1 class="title">{displayName(m)}</h1>
        {#if displayName(m) !== m.title}
          <p class="orig">{m.title}</p>
        {/if}

        <div class="scoreline">
          {#if m.score}
            <span class="big-score">{m.score.toFixed(2)}</span><span class="of">/ 10</span>
            <span class="vr"></span>
          {/if}
          <span class="meta">
            {metaLine}{#if author}&nbsp;·
              <a class="meta-link" href={`/manga?author=${encodeURIComponent(author)}`} title={`Toate titlurile de ${author}`}>{author}</a>{/if}
          </span>
        </div>

        {#if m.genres?.length}
          <div class="pills">
            {#each m.genres as g (g)}
              <a class="pill" href={`/manga?gen=${encodeURIComponent(g)}`}>{genreRo(g)}</a>
            {/each}
          </div>
        {/if}

        {#if displaySynopsis(m)}<p class="syn">{displaySynopsis(m)}</p>{/if}

        {#if editOpen}
          <div class="edit-panel">
            <label class="ep-field">
              <span>Titlu românesc</span>
              <input bind:value={editTitleRo} placeholder={m.title} />
            </label>
            <label class="ep-field">
              <span>Descriere în română</span>
              <textarea bind:value={editSynRo} rows="5" placeholder="Descrierea afișată vizitatorilor…"></textarea>
            </label>
            <div class="ep-actions">
              {#if canPoster}
                <label class="btn ghost ep-poster" class:busy={posterBusy}>
                  {posterBusy ? 'Se încarcă…' : '↑ Înlocuiește posterul'}
                  <input type="file" accept="image/*" onchange={replacePoster} disabled={posterBusy} />
                </label>
              {/if}
              <span class="ep-spacer"></span>
              <button class="btn ghost" onclick={() => (editOpen = false)}>Anulează</button>
              <button class="btn fill" onclick={saveEdit} disabled={editSaving}>
                {editSaving ? 'Se salvează…' : 'Salvează'}
              </button>
            </div>
          </div>
        {/if}

        <div class="actions">
          {#if data.chapters.length}
            <a class="btn fill" href={`/manga/${titleRef(m)}/chapter/${chapNum(data.chapters[0].chapterNumber)}`}>
              📖 Citește
            </a>
          {/if}
          <button class="btn ghost" class:on={inReadlist} onclick={toggleList} disabled={listBusy}>
            {inReadlist ? '✓ În readlist' : '+ Readlist'}
          </button>
          <StatusPicker
            value={rStatus}
            options={R_LABELS}
            busy={listBusy}
            onselect={(s) => setStatus(s as RStatus)}
            onclear={clearStatus}
          />
          <div class="rate" onmouseleave={() => (hoverStar = 0)} role="group" aria-label="Evaluează">
            <span class="rate-t">{rateText}</span>
            <span class="rate-stars">
              {#each [1, 2, 3, 4, 5] as n (n)}
                <button
                  class="star"
                  class:on={(hoverStar || stars) >= n}
                  aria-label={`${n} din 5`}
                  onmouseenter={() => (hoverStar = n)}
                  onclick={() => rate(n)}
                >★</button>
              {/each}
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</section>

<!-- TABS -->
<div class="container body">
  <div class="tabs" role="tablist">
    <button class="tab" class:on={tab === 'capitole'} role="tab" aria-selected={tab === 'capitole'} onclick={() => (tab = 'capitole')}>
      Capitole
    </button>
    <button class="tab" class:on={tab === 'recenzii'} role="tab" aria-selected={tab === 'recenzii'} onclick={() => (tab = 'recenzii')}>
      Recenzii
    </button>
    <button class="tab" class:on={tab === 'comentarii'} role="tab" aria-selected={tab === 'comentarii'} onclick={() => (tab = 'comentarii')}>
      Comentarii
    </button>
  </div>

  {#if tab === 'capitole'}
    {#if data.chapters.length}
      <div class="chs">
        {#each data.chapters as ch (ch.id)}
          <a class="ch" href={`/manga/${titleRef(m)}/chapter/${chapNum(ch.chapterNumber)}`}>
            <span class="ch-n">{chapNum(ch.chapterNumber)}</span>
            <span class="ch-main">
              <span class="ch-t">{ch.title ?? `Capitolul ${chapNum(ch.chapterNumber)}`}</span>
              <!-- No "Subtitrare RO" fallback: the whole site is Romanian, so
                   saying it on every chapter row was noise. -->
              {#if ch.pages}<span class="ch-m">{ch.pages} pagini</span>{/if}
            </span>
            <span class="ch-go">→</span>
          </a>
        {/each}
      </div>
    {:else}
      <p class="empty">Niciun capitol disponibil încă.</p>
    {/if}
  {:else if tab === 'recenzii'}
    <div class="disc">
      <!-- Review composer -->
      {#if auth.isAuthenticated}
        <div class="rev-compose">
          <div class="rev-compose-head">
            <span class="rev-label">Recenzia ta</span>
            <span class="rate-stars">
              {#each [1, 2, 3, 4, 5] as n (n)}
                <button
                  class="star star-sm"
                  class:on={(hoverStar || stars) >= n}
                  aria-label={`${n} din 5`}
                  onmouseenter={() => (hoverStar = n)}
                  onmouseleave={() => (hoverStar = 0)}
                  onclick={() => rate(n)}
                >★</button>
              {/each}
            </span>
          </div>
          <textarea
            class="rev-input"
            bind:this={reviewEl}
            bind:value={myReview}
            placeholder="Ce părere ai despre această manga?"
            rows="4"
            maxlength="4000"
          ></textarea>
          <div class="rev-compose-foot">
            <div class="rev-foot-left">
              <EmojiPicker onPick={(e) => (myReview += e)} />
              <SpoilerButton bind:value={myReview} input={reviewEl} />
              <GifPicker onPick={(url) => (myReview = myReview ? `${myReview} ${url}` : url)} />
              <span class="rev-count">{myReview.length}/4000</span>
            </div>
            <button class="btn fill btn-rev" onclick={submitReview} disabled={savingReview || !myReview.trim()}>
              {savingReview ? 'Se publică...' : 'Publică recenzia'}
            </button>
          </div>
        </div>
      {:else}
        <p class="rev-login"><a href={`/login?redirect=/manga/${m.id}`}>Conectează-te</a> pentru a scrie o recenzie.</p>
      {/if}

      <!-- Reviews list -->
      {#if !reviewsLoaded}
        <p class="empty">Se încarcă recenziile...</p>
      {:else if reviews.length === 0}
        <p class="empty">Nicio recenzie deocamdată. Fii primul care scrie una!</p>
      {:else}
        <div class="rev-list">
          {#each reviews as r (r.entryId)}
            <article class="rev">
              <a class="rev-avatar" class:monogram={!r.user.avatarUrl} style={`--mg-hue:${nameHue(r.user.username)}`} href={`/user/${r.user.username}`}>
                {#if r.user.avatarUrl}
                  <img src={api.resolveUrl(r.user.avatarUrl)} alt={r.user.username} />
                {:else}
                  <span>{r.user.username.charAt(0).toUpperCase()}</span>
                {/if}
              </a>
              <div class="rev-body">
                <div class="rev-head">
                  <a class="rev-user" href={`/user/${r.user.username}`}>{r.user.username}</a>
                  {#if r.score}
                    <span class="rev-stars">{'★'.repeat(Math.round(r.score / 2))}<span class="rev-stars-off">{'★'.repeat(5 - Math.round(r.score / 2))}</span></span>
                  {/if}
                  <span class="rev-date">{reviewDate(r.updatedAt)}</span>
                </div>
                <p class="rev-text"><RichText text={r.notes} /></p>
                <a class="rev-replies-btn" href={`/manga/${titleRef(m)}/review/${r.entryId}`}>
                  💬 {r.replyCount ? `${r.replyCount} comentarii` : 'Comentează'} →
                </a>
              </div>
            </article>
          {/each}
        </div>
      {/if}
    </div>
  {:else}
    <div class="disc">
      <!-- Scope: whole series or one chapter -->
      {#if data.chapters.length}
        <div class="scope">
          <span class="scope-label">Arată comentarii pentru</span>
          <button class="scope-pill" class:on={commentChapterId === 0} onclick={() => (commentChapterId = 0)}>
            Toată seria
          </button>
          <select
            class="scope-select"
            class:on={commentChapterId !== 0}
            bind:value={commentChapterId}
          >
            <option value={0} disabled>Un capitol…</option>
            {#each data.chapters as ch (ch.id)}
              <option value={ch.id}>Capitolul {chapNum(ch.chapterNumber)}</option>
            {/each}
          </select>
        </div>
      {/if}
      {#key commentChapterId}
        <CommentSection
          mangaId={m.id}
          chapterId={commentChapterId || undefined}
          heading={commentChapterId
            ? `Comentarii · Capitolul ${chapNum(data.chapters.find((c) => c.id === commentChapterId)?.chapterNumber ?? '')}`
            : 'Comentarii · Toată seria'}
        />
      {/key}
    </div>
  {/if}

  {#if data.similar.length}
    <section class="sim">
      <div class="sim-head"><h2>Similare</h2></div>
      <PosterGrid items={data.similar} kind="manga" />
    </section>
  {/if}
</div>

<style>
  /* no overflow:hidden — the Marchează menu must be able to drop past the edge */
  .hero { position: relative; }
  .hero-bg {
    position: absolute; inset: 0;
    background-size: cover; background-position: center 20%;
    filter: blur(42px) saturate(1.1); opacity: 0.25; transform: scale(1.2);
  }
  .hero-fade {
    position: absolute; inset: 0;
    background:
      radial-gradient(1000px 420px at 18% 0%, color-mix(in srgb, var(--accent-strong) 12%, transparent), transparent 60%),
      linear-gradient(to top, var(--surface-base), transparent 55%);
  }
  .hero-in { position: relative; padding-block: var(--space-6); }
  .crumb {
    display: inline-block; margin-bottom: var(--space-5);
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
  }
  .crumb:hover { color: var(--text-primary); }

  .hero-grid { display: grid; grid-template-columns: 240px 1fr; gap: var(--space-6); align-items: start; }
  .poster img {
    width: 100%; aspect-ratio: 2/3; object-fit: cover;
    border-radius: var(--radius-lg); border: 1px solid var(--border-default);
    box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
  }

  .badges { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .type-badge {
    font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.08em; text-transform: uppercase;
    padding: 4px 9px; border-radius: var(--radius-sm);
    background: var(--surface-raised); border: 1px solid var(--border-default); color: var(--text-muted);
  }
  .status { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .status.live { color: var(--success); }

  .title { font-size: clamp(1.9rem, 1.5rem + 1.6vw, 2.5rem); line-height: 1.06; letter-spacing: -0.015em; margin-top: 14px; }
  .orig { font-family: var(--font-mono); font-size: var(--fs-small); color: var(--text-muted); margin-top: 4px; }

  .scoreline { display: flex; align-items: center; gap: 18px; margin-block: 18px; flex-wrap: wrap; }
  .big-score { font-family: var(--font-display); font-size: 1.875rem; font-weight: var(--fw-semibold); color: var(--accent); }
  .of { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); margin-left: -12px; }
  .vr { width: 1px; height: 30px; background: var(--border-default); }
  .meta { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .meta-link { color: var(--text-muted); text-decoration: underline; text-underline-offset: 3px; text-decoration-color: var(--border-default); }
  .meta-link:hover { color: var(--accent); text-decoration-color: currentColor; }

  .pills { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 20px; }
  .pill {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    border: 1px solid var(--border-default); padding: 4px 11px; border-radius: var(--radius-pill);
  }
  .pill:hover { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent); }

  .syn { font-size: var(--fs-body); line-height: 1.65; color: var(--text-muted); max-width: 620px; margin-bottom: 24px; }

  .edit-btn {
    margin-left: 6px; padding: 3px 10px; border-radius: var(--radius-sm);
    border: 1px solid var(--border-default); background: transparent;
    color: var(--text-muted); cursor: pointer;
    font-family: var(--font-mono); font-size: var(--fs-caption);
  }
  .edit-btn:hover, .edit-btn.on { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent); }

  .edit-panel {
    margin: 14px 0 4px; padding: 16px;
    background: var(--surface-raised); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); max-width: 640px;
  }
  .ep-field { display: block; margin-bottom: 12px; }
  .ep-field span {
    display: block; margin-bottom: 5px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }
  .ep-field input, .ep-field textarea {
    width: 100%; padding: 9px 12px;
    background: var(--surface-inset, var(--surface-overlay)); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); outline: none;
    font-size: var(--fs-small); font-family: var(--font-body); resize: vertical;
  }
  .ep-field input:focus, .ep-field textarea:focus { border-color: var(--accent); }
  .ep-actions { display: flex; align-items: center; gap: 9px; }
  .ep-actions .btn { padding: 9px 16px; font-size: var(--fs-caption); }
  .ep-spacer { flex: 1; }
  .ep-poster { position: relative; overflow: hidden; display: inline-flex; }
  .ep-poster input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
  .ep-poster.busy { opacity: 0.6; }

  .actions { display: flex; align-items: center; gap: 11px; flex-wrap: wrap; }
  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 13px 24px; border-radius: var(--radius-md); cursor: pointer; white-space: nowrap;
  }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-raised); }
  .btn.ghost.on {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .btn:disabled { opacity: 0.6; cursor: wait; }


  .rate { display: flex; align-items: center; gap: 10px; margin-left: 6px; }
  .rate-t { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .rate-stars { display: flex; gap: 3px; }
  .star {
    background: none; border: none; padding: 0; cursor: pointer;
    font-size: 1.3125rem; line-height: 1; color: var(--border-default);
    transition: color var(--motion-fast) var(--ease), transform var(--motion-fast) var(--ease);
  }
  .star.on { color: var(--accent); }
  .star:hover { transform: scale(1.15); }

  .body { padding-block: var(--space-2) var(--space-8); }
  .tabs { display: flex; border-bottom: 1px solid var(--border-subtle); margin-bottom: var(--space-5); }
  .tab {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-muted);
    background: none; border: none; cursor: pointer;
    padding: 12px 16px; border-bottom: 2px solid transparent; margin-bottom: -1px;
  }
  .tab:hover { color: var(--text-primary); }
  .tab.on { color: var(--text-primary); border-bottom-color: var(--accent); }

  .chs { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
  .ch {
    display: flex; align-items: center; gap: 15px;
    padding: 13px 16px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised);
    transition: border-color var(--motion-base) var(--ease), background var(--motion-base) var(--ease);
  }
  .ch:hover { border-color: var(--border-default); background: var(--surface-overlay); }
  .ch-n {
    font-family: var(--font-display); font-size: 1.25rem; font-weight: var(--fw-semibold);
    color: var(--text-muted); min-width: 2.2ch; text-align: right;
  }
  .ch-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .ch-t {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .ch-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 3px; }
  .ch-go { color: var(--text-muted); }
  .ch:hover .ch-go { color: var(--accent); }

  .empty { color: var(--text-muted); padding: var(--space-6) 0; }
  .disc { max-width: 840px; }

  /* ---- reviews ---- */
  .rev-compose {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised); padding: 16px; margin-bottom: var(--space-6);
  }
  .rev-compose-head { display: flex; align-items: center; justify-content: space-between; margin-bottom: 10px; }
  .rev-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: 600;
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted);
  }
  .star-sm { font-size: 1.0625rem; }
  .rev-input {
    width: 100%; padding: 10px 12px;
    background: var(--surface-inset); border: 1px solid var(--border-default); border-radius: var(--radius-sm);
    color: var(--text-primary); font-family: var(--font-body); font-size: var(--fs-body); line-height: 1.6;
    resize: vertical;
  }
  .rev-input::placeholder { color: var(--text-faint); }
  .rev-input:focus { outline: none; border-color: var(--accent); box-shadow: 0 0 0 3px var(--focus-ring); }
  .rev-compose-foot { display: flex; align-items: center; justify-content: space-between; margin-top: 10px; }
  .rev-foot-left { display: flex; align-items: center; gap: 10px; }
  .rev-count { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .btn-rev { padding: 9px 20px; font-size: var(--fs-caption); }
  .rev-login { color: var(--text-muted); font-size: var(--fs-small); margin-bottom: var(--space-5); }

  .rev-list { margin-top: 4px; }
  .rev { display: flex; gap: 14px; padding: 20px 0; border-top: 1px solid var(--border-subtle); }
  .rev-avatar {
    flex-shrink: 0; width: 36px; height: 36px; border-radius: 50%; overflow: hidden;
    background: var(--surface-overlay); border: 1px solid var(--border-subtle);
    display: flex; align-items: center; justify-content: center;
    font-family: var(--font-mono); font-size: var(--fs-caption); font-weight: 600; color: var(--text-muted);
  }
  .rev-avatar:hover { color: #fff; }
  .rev-avatar img { width: 100%; height: 100%; object-fit: cover; }
  .rev-body { flex: 1; min-width: 0; }
  .rev-head { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; margin-bottom: 4px; }
  .rev-user { font-size: var(--fs-body); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .rev-user:hover { color: var(--accent); }
  .rev-stars { font-size: 0.875rem; color: var(--accent); letter-spacing: 1px; }
  .rev-stars-off { color: var(--border-default); }
  .rev-date { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .rev-text {
    font-size: var(--fs-body); line-height: 1.65; color: var(--text-muted);
    white-space: pre-wrap; word-break: break-word; margin: 0;
  }
  .rev-replies-btn {
    display: inline-block; margin-top: 8px; padding: 4px 0;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--accent);
    transition: color var(--motion-fast) var(--ease);
  }
  .rev-replies-btn:hover { color: var(--accent-hover); }

  /* ---- comment scope ---- */
  .scope { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; margin-bottom: var(--space-5); }
  .scope-label { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.1em; }
  .scope-pill, .scope-select {
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    background: transparent; border: 1px solid var(--border-default); border-radius: var(--radius-pill);
    padding: 5px 14px; cursor: pointer;
  }
  .scope-pill:hover, .scope-select:hover { color: var(--text-primary); }
  .scope-pill.on, .scope-select.on {
    color: var(--accent);
    border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .scope-select option { background: var(--surface-raised); color: var(--text-primary); }

  .sim { margin-top: var(--space-8); }
  .sim-head {
    display: flex; align-items: baseline; justify-content: space-between;
    border-bottom: 1px solid var(--border-subtle); padding-bottom: var(--space-3); margin-bottom: var(--space-5);
  }
  .sim-head h2 { font-size: var(--fs-h2); }

  @media (max-width: 760px) {
    .hero-grid { grid-template-columns: 130px 1fr; gap: var(--space-4); }
    .syn { display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
    .chs { grid-template-columns: minmax(0, 1fr); }
  }
  @media (max-width: 520px) {
    .hero-grid { grid-template-columns: minmax(0, 1fr); }
    .poster { max-width: 170px; }
  }
</style>
