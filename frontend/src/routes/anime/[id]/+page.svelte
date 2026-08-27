<script lang="ts">
  import GifPicker from '$lib/components/GifPicker.svelte';
  import SpoilerButton from '$lib/components/SpoilerButton.svelte';
  import RichText from '$lib/components/RichText.svelte';
  import { goto, invalidateAll } from '$app/navigation';
  import PosterGrid from '$lib/components/PosterGrid.svelte';
  import CommentSection from '$lib/components/CommentSection.svelte';
  import EmojiPicker from '$lib/components/EmojiPicker.svelte';
  import StatusPicker from '$lib/components/StatusPicker.svelte';
  import PagePicker from '$lib/components/PagePicker.svelte';
  import { authStore } from '$lib/stores/auth';
  import api from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import { toast } from '$lib/stores/toast';
  import { displayName, displaySynopsis, genreRo, seasonYearLabel, studioOf, titleRef } from '$lib/types';
  import type { Review } from '$shared/types';

  let { data } = $props();
  const a = $derived(data.anime);
  const auth = $derived($authStore);

  // AniList's relation vocabulary in Romanian. SEQUEL/PREQUEL never reach the
  // grid — those are the season strip — so they are absent on purpose.
  const RELATION_LABEL: Record<string, string> = {
    ALTERNATIVE: 'Versiune alternativă',
    ALTERNATIVE_VERSION: 'Versiune alternativă',
    SIDE_STORY: 'Poveste secundară',
    PARENT: 'Seria principală',
    SPIN_OFF: 'Spin-off',
    SUMMARY: 'Rezumat'
  };

  let tab = $state<'episoade' | 'recenzii' | 'comentarii'>('episoade');
  let listBusy = $state(false);
  let inList = $state(false);
  let stars = $state(0); // 1-5 (backend score is 1-10)
  let hoverStar = $state(0);

  type WStatus = 'watching' | 'completed' | 'plan-to-watch' | 'on-hold' | 'dropped';
  const W_LABELS: [WStatus, string][] = [
    ['watching', 'În vizionare'],
    ['completed', 'Văzut'],
    ['plan-to-watch', 'În plan'],
    ['on-hold', 'În așteptare'],
    ['dropped', 'Abandonat']
  ];
  // '' = not in list; otherwise the entry's real status — sent back on every
  // upsert so a rating or review never clobbers it
  let wStatus = $state<WStatus | ''>('');

  // reviews
  let reviews = $state<Review[]>([]);
  let reviewsLoaded = $state(false);
  let myReview = $state('');
  let reviewEl = $state<HTMLTextAreaElement | null>(null);
  let savingReview = $state(false);

  /* Comment scope: always an episode id. The series-wide thread was dropped —
     discussion is about what happened in an episode, and the "Tot serialul"
     option mostly collected spoilers with no context. Defaults to episode 1.
     Reset on navigation: SvelteKit reuses this component between series, so
     $state alone would carry the previous title's episode id over. */
  let commentEpisodeId = $state(0);
  let scopedFor = $state(0);
  $effect(() => {
    if (scopedFor === a.id) return;
    scopedFor = a.id;
    commentEpisodeId = data.episodeIndex[0]?.id ?? 0;
  });

  const statusLabel = (s: string | null | undefined) =>
    s === 'airing' ? 'În difuzare' : s === 'completed' ? 'Finalizat' : s === 'upcoming' ? 'În curând' : (s ?? '—');

  // studio renders separately as a link into the filtered catalog
  const metaLine = $derived(
    [seasonYearLabel(a), `${a.episodes ?? '?'} episoade`].filter(Boolean).join(' · ')
  );
  const studio = $derived(studioOf(a));

  // inline catalog edit (coordinator/admin, translators too — same backend gate)
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
    editTitleRo = a.titleRomanian ?? '';
    editSynRo = a.synopsisRomanian ?? '';
    editOpen = !editOpen;
  }

  async function saveEdit() {
    editSaving = true;
    try {
      await api.patchAnime(a.id, {
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
      await api.uploadPoster('anime', a.id, file);
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
      goto(`/login?redirect=/anime/${a.id}`);
      return false;
    }
    return true;
  }

  // the watchlist button is the "de văzut" shelf (plan-to-watch), separate
  // from the status marker — marking "Văzut" etc. must not light it up
  const inWatchlist = $derived(wStatus === 'plan-to-watch');

  async function toggleList() {
    if (!requireAuth()) return;
    listBusy = true;
    try {
      if (inWatchlist) {
        await api.removeFromWatchlist(a.id);
        inList = false;
        wStatus = '';
        stars = 0;
        toast.success('Eliminat din watchlist.');
      } else {
        if (inList) await api.updateWatchlistEntry(a.id, { status: 'plan-to-watch' });
        else await api.addToWatchlist(a.id, { status: 'plan-to-watch' });
        inList = true;
        wStatus = 'plan-to-watch';
        toast.success('Adăugat în watchlist!');
      }
    } catch {
      toast.error('Eroare la actualizarea watchlist-ului.');
    } finally {
      listBusy = false;
    }
  }

  async function setStatus(next: WStatus) {
    if (!requireAuth()) return;
    listBusy = true;
    try {
      const form = {
        status: next,
        ...(next === 'completed' && a.episodes ? { episodesWatched: a.episodes } : {}),
        ...(stars ? { score: stars * 2 } : {})
      };
      if (inList) await api.updateWatchlistEntry(a.id, form);
      else await api.addToWatchlist(a.id, form);
      inList = true;
      wStatus = next;
      toast.success(`Status: ${W_LABELS.find(([s]) => s === next)?.[1] ?? next}.`);
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
      await api.removeFromWatchlist(a.id);
      inList = false;
      wStatus = '';
      stars = 0;
      toast.success('Eliminat din listă.');
    } catch {
      toast.error('Eroare la actualizare.');
    } finally {
      listBusy = false;
    }
  }

  /** Set a rating. `n` is 1..5; the backend scale is 1..10.
   *
   *  Removing is a separate control, not a second meaning for the same star.
   *  Click-the-star-again was tried and removed: which action a star performed
   *  depended on whether it happened to equal the current rating, so the
   *  meaning of the control changed as the pointer crossed it. A one-pixel
   *  move should not turn "rate 4" into "erase everything". */
  async function rate(n: number) {
    if (!requireAuth()) return;
    const prev = stars;
    stars = n;
    try {
      const form = { status: wStatus || ('plan-to-watch' as const), score: n * 2 };
      if (inList) await api.updateWatchlistEntry(a.id, form);
      else await api.addToWatchlist(a.id, form);
      inList = true;
    } catch {
      stars = prev;
      toast.error('Eroare la salvarea notei.');
    }
  }

  /** Remove the rating without touching status or progress (`score: 0`). */
  async function clearRating() {
    if (!requireAuth() || !stars) return;
    const prev = stars;
    stars = 0;
    try {
      await api.updateWatchlistEntry(a.id, {
        status: wStatus || ('plan-to-watch' as const),
        score: 0
      });
    } catch {
      stars = prev;
      toast.error('Eroare la ștergerea notei.');
    }
  }

  async function loadReviews() {
    try {
      const res = await api.getAnimeReviews(a.id);
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
        status: wStatus || ('plan-to-watch' as const),
        notes: text,
        ...(stars ? { score: stars * 2 } : {})
      };
      if (inList) await api.updateWatchlistEntry(a.id, form);
      else await api.addToWatchlist(a.id, form);
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
    if (auth.isAuthenticated && a.id) {
      api
        .getWatchlistEntry(a.id)
        .then((res) => {
          inList = true;
          wStatus = res.data.status as WStatus;
          stars = res.data.score ? Math.round(res.data.score / 2) : 0;
          if (res.data.notes) myReview = res.data.notes;
        })
        .catch(() => {
          inList = false;
          wStatus = '';
          stars = 0;
        });
    }
  });

  const epLabel = (e: (typeof data.episodes)[number]) =>
    e.title ? `${e.episodeNumber}. ${e.title}` : `Episodul ${e.episodeNumber}`;

  /* Filler and recap, MAL's marks. Red text plus a word, not colour alone:
     ~8% of men cannot reliably separate red from the surrounding grey, and
     "which episodes can I skip" is exactly the question this answers. */
  const epTag = (e: (typeof data.episodes)[number]) =>
    e.isFiller ? 'filler' : e.isRecap ? 'recap' : null;
</script>

<svelte:head><title>{displayName(a)} · Anime-Kage</title></svelte:head>

<!-- HERO -->
<section class="hero">
  {#if a.imageUrl}
    <div class="hero-bg" style={`background-image:url(${api.resolveUrl(a.imageUrl)})`}></div>
  {/if}
  <div class="hero-fade"></div>
  <div class="container hero-in">
    <a class="crumb" href="/anime">← Catalog</a>
    <div class="hero-grid">
      <div class="poster">
        {#if a.imageUrl}<img src={api.resolveUrl(a.imageUrl)} alt={displayName(a)} width="240" height="360" />{/if}
      </div>

      <div class="head">
        <div class="badges">
          <span class="type-badge">{a.type ?? 'TV'}</span>
          <span class="status" class:live={a.status === 'airing'}>{statusLabel(a.status)}</span>
          {#if canEdit}
            <button class="edit-btn" class:on={editOpen} onclick={openEdit} title="Editează seria">✎ Editează</button>
          {/if}
        </div>
        <h1 class="title">{displayName(a)}</h1>
        {#if displayName(a) !== a.title}
          <p class="orig">{a.title}</p>
        {/if}

        <div class="scoreline">
          {#if a.score}
            <span class="big-score">{a.score.toFixed(2)}</span><span class="of">/ 10</span>
            <span class="vr"></span>
          {/if}
          <span class="meta">
            {metaLine}{#if studio}&nbsp;·
              <a class="meta-link" href={`/anime?studio=${encodeURIComponent(studio)}`} title={`Tot ce a produs ${studio}`}>{studio}</a>{/if}
          </span>
        </div>

        {#if a.genres?.length}
          <div class="pills">
            {#each a.genres as g (g)}
              <a class="pill" href={`/anime?gen=${encodeURIComponent(g)}`}>{genreRo(g)}</a>
            {/each}
          </div>
        {/if}

        {#if displaySynopsis(a)}<p class="syn">{displaySynopsis(a)}</p>{/if}

        {#if editOpen}
          <div class="edit-panel">
            <label class="ep-field">
              <span>Titlu românesc</span>
              <input bind:value={editTitleRo} placeholder={a.title} />
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
          {#if data.episodeIndex.length}
            <a class="btn fill" href={`/anime/${titleRef(a)}/episode/${data.episodeIndex[0].episodeNumber}`}>
              ▶ Vizionează
            </a>
          {/if}
          <button class="btn ghost" class:on={inWatchlist} onclick={toggleList} disabled={listBusy}>
            {inWatchlist ? '✓ În watchlist' : '+ Watchlist'}
          </button>
          <StatusPicker
            value={wStatus}
            options={W_LABELS}
            busy={listBusy}
            onselect={(s) => setStatus(s as WStatus)}
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
                  title={`${n} din 5`}
                  onmouseenter={() => (hoverStar = n)}
                  onclick={() => rate(n)}
                >★</button>
              {/each}
              {#if stars}
                <button
                  class="rate-clear"
                  aria-label="Șterge nota"
                  title="Șterge nota"
                  onclick={clearRating}
                >×</button>
              {/if}
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
    <button
      class="tab"
      class:on={tab === 'episoade'}
      role="tab"
      aria-selected={tab === 'episoade'}
      onclick={() => (tab = 'episoade')}
    >
      Episoade
    </button>
    <button
      class="tab"
      class:on={tab === 'recenzii'}
      role="tab"
      aria-selected={tab === 'recenzii'}
      onclick={() => (tab = 'recenzii')}
    >
      Recenzii
    </button>
    <button
      class="tab"
      class:on={tab === 'comentarii'}
      role="tab"
      aria-selected={tab === 'comentarii'}
      onclick={() => (tab = 'comentarii')}
    >
      Comentarii
    </button>
  </div>

  {#if tab === 'episoade'}
    <!-- Season strip, Crunchyroll's placement: directly above the episode list,
         so switching season and picking an episode are the same gesture. Only
         drawn when the series actually has neighbouring seasons in the
         catalog — a standalone film gets nothing. -->
    {#if data.relations.chain.length > 1}
      <nav class="seasons" aria-label="Sezoane">
        {#each data.relations.chain as s, i (s.id)}
          {@const current = s.id === a.id}
          <a
            class="season"
            class:on={current}
            href={`/anime/${s.id}`}
            aria-current={current ? 'page' : undefined}
            title={displayName(s)}
          >
            <span class="season-n">Sezonul {i + 1}</span>
            <span class="season-m">
              {s.year ?? '—'}{#if s.episodeCount}{' · '}{s.episodeCount} ep{:else}{' · '}în curând{/if}
            </span>
          </a>
        {/each}
      </nav>
    {/if}

    {#if data.episodes.length}
      <div class="eps">
        {#each data.episodes as e (e.id)}
          <a class="ep" href={`/anime/${titleRef(a)}/episode/${e.episodeNumber}`}>
            <span class="ep-thumb" style={a.imageUrl ? `background-image:url(${api.resolveUrl(a.imageUrl)})` : ''}>
              <span class="ep-play">▶</span>
            </span>
            <span class="ep-main">
              <span class="ep-t" class:filler={e.isFiller} class:recap={e.isRecap}>
                {epLabel(e)}{#if epTag(e)}<span class="ep-tag">({epTag(e)})</span>{/if}
              </span>
              <!-- Only the "no sources yet" case gets a second line. Listing
                   the hosts here made every row two lines of noise for
                   something you only act on inside the episode; an episode
                   that cannot be played yet is the one thing worth saying
                   before the click. -->
              {#if !e.links?.length}
                <span class="ep-m">în curând</span>
              {/if}
            </span>
            <span class="ep-n">{e.episodeNumber}</span>
          </a>
        {/each}
      </div>

      <!-- Same pager as the catalog (/anime): a URL param, so a page of a long
           run is linkable and the back button works. `ep` rather than `page`
           because the two could coexist on this route later. -->
      {#if data.epPages > 1}
        <nav class="pager">
          {#if data.epPage > 1}
            <a class="pill" href={`?ep=${data.epPage - 1}`}>← Anterioare</a>
          {/if}
          <!-- The real episode numbers on this page, not positions in the
               list: runs have gaps (One Piece is missing 1164, so 1167
               episodes span numbers 1–1168) and a positional label would
               contradict the numbers right next to it. -->
          <span class="pager-range">
            episoadele {data.episodes[0].episodeNumber}–{data.episodes[data.episodes.length - 1]
              .episodeNumber}
          </span>
          <PagePicker page={data.epPage} pages={data.epPages} hrefFor={(n) => `?ep=${n}`} />
          {#if data.epPage < data.epPages}
            <a class="pill" href={`?ep=${data.epPage + 1}`}>Următoarele →</a>
          {/if}
        </nav>
      {/if}
    {:else}
      <p class="empty">Niciun episod disponibil încă.</p>
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
            placeholder="Ce părere ai despre acest anime?"
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
        <p class="rev-login"><a href={`/login?redirect=/anime/${a.id}`}>Conectează-te</a> pentru a scrie o recenzie.</p>
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
                <a class="rev-replies-btn" href={`/anime/${titleRef(a)}/review/${r.entryId}`}>
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
      <!-- Scope: whole series or one episode -->
      {#if data.episodeIndex.length}
        <div class="scope">
          <label class="scope-label" for="ep-scope">Comentarii pentru</label>
          <select id="ep-scope" class="scope-select" bind:value={commentEpisodeId}>
            {#each data.episodeIndex as e (e.id)}
              <option value={e.id}>Episodul {e.episodeNumber}</option>
            {/each}
          </select>
        </div>
      {/if}
      {#key commentEpisodeId}
        <CommentSection
          animeId={a.id}
          episodeId={commentEpisodeId || undefined}
          heading={`Comentarii · Episodul ${data.episodeIndex.find((e) => e.id === commentEpisodeId)?.episodeNumber ?? 1}`}
        />
      {/key}
    </div>
  {/if}

  <!-- Everything in the franchise that isn't a season: alternative retellings,
       spin-offs, side stories. This is the Fate case — Unlimited Blade Works
       and Heaven's Feel are not "season 2" of anything, and the relation data
       says so, so they get a grid of their own rather than a place in the
       season strip. Above "Similare" because a sibling series is a much
       stronger link than a shared genre. -->
  {#if data.relations.related.length}
    <section class="sim">
      <div class="sim-head">
        <h2>Din aceeași serie</h2>
      </div>
      <div class="fran">
        {#each data.relations.related as rel (rel.id)}
          <a class="fr" href={`/anime/${rel.id}`}>
            <span
              class="fr-art media-tone"
              style={rel.imageUrl ? `background-image:url(${api.resolveUrl(rel.imageUrl)})` : ''}
            ></span>
            <span class="fr-body">
              <span class="fr-kind">{RELATION_LABEL[rel.relation] ?? 'Înrudit'}</span>
              <span class="fr-t">{displayName(rel)}</span>
              <span class="fr-m">
                {rel.year ?? '—'} · {(rel.type ?? 'TV').toUpperCase()}
              </span>
            </span>
          </a>
        {/each}
      </div>
    </section>
  {/if}

  {#if data.similar.length}
    <section class="sim">
      <div class="sim-head">
        <h2>Similare</h2>
      </div>
      <PosterGrid items={data.similar} />
    </section>
  {/if}
</div>

<style>
  /* ---- hero ---- */
  /* no overflow:hidden — the Marchează menu must be able to drop past the edge */
  /* the blurred backdrop is scale(1.2), so it must be clipped to the hero —
     otherwise it bleeds ~10% past each edge and scrolls the page sideways */
  .hero { position: relative; overflow: hidden; }
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
  .hero-in { position: relative; padding-block: var(--space-6) var(--space-6); }
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

  /* episode pager — same rules as the catalog's, so the two read as one
     control rather than two similar-looking ones */
  .pager {
    display: flex; align-items: center; justify-content: center; gap: 14px;
    margin-top: var(--space-6);
  }
  .pager-range { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .pager .pill { background: var(--surface-raised); border: 1px solid var(--border-subtle); }
  .pager .pill:hover { border-color: var(--accent); }

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
  /* Sits after the stars and only exists once there is something to remove.
     Quiet until hovered, so it never competes with the rating itself. */
  .rate-clear {
    margin-left: 6px; padding: 0 4px; line-height: 1;
    background: none; border: 0; cursor: pointer;
    font-size: 1.05em; color: var(--text-faint);
    transition: color 120ms ease;
  }
  .rate-clear:hover { color: var(--danger); }
  .rate-clear:focus-visible { outline: 2px solid var(--focus-ring); outline-offset: 2px; border-radius: 3px; }
  .rate-t { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .rate-stars { display: flex; gap: 3px; }
  .star {
    background: none; border: none; padding: 0; cursor: pointer;
    font-size: 1.3125rem; line-height: 1; color: var(--border-default);
    transition: color var(--motion-fast) var(--ease), transform var(--motion-fast) var(--ease);
  }
  .star.on { color: var(--accent); }
  .star:hover { transform: scale(1.15); }

  /* ---- tabs + episodes ---- */
  .body { padding-block: var(--space-2) var(--space-8); }
  .tabs { display: flex; border-bottom: 1px solid var(--border-subtle); margin-bottom: var(--space-5); }
  .tab {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-muted);
    background: none; border: none; cursor: pointer;
    padding: 12px 16px; border-bottom: 2px solid transparent; margin-bottom: -1px;
  }
  .tab:hover { color: var(--text-primary); }
  .tab.on { color: var(--text-primary); border-bottom-color: var(--accent); }

  /* ---- season strip ---- */
  /* Scrolls sideways rather than wrapping: a long-running series would
     otherwise push the episode list a screen further down. */
  .seasons {
    display: flex; gap: 9px; overflow-x: auto; overflow-y: hidden;
    padding-bottom: 4px; margin-bottom: var(--space-5);
  }
  .season {
    flex: 0 0 auto; display: flex; flex-direction: column; gap: 3px;
    padding: 9px 15px; border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md); background: var(--surface-raised);
    transition: border-color var(--motion-fast) var(--ease);
  }
  .season:hover { border-color: var(--border-default); }
  .season.on { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, transparent); }
  .season-n { font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary); white-space: nowrap; }
  .season.on .season-n { color: var(--accent); }
  .season-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); white-space: nowrap; }

  .eps { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
  .ep {
    display: flex; align-items: center; gap: 15px;
    padding: 13px 16px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised);
    transition: border-color var(--motion-base) var(--ease), background var(--motion-base) var(--ease);
  }
  .ep:hover { border-color: var(--border-default); background: var(--surface-overlay); }
  .ep-thumb {
    position: relative; width: 64px; height: 38px; border-radius: var(--radius-sm); flex: 0 0 auto;
    background-color: var(--surface-overlay); background-size: cover; background-position: center 20%;
    display: grid; place-items: center; overflow: hidden;
  }
  .ep-play { color: #fff; font-size: 0.8125rem; text-shadow: 0 1px 4px rgba(0, 0, 0, 0.8); }
  .ep-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .ep-t {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  /* Filler is the loud one: it's the episode you can skip. Recap gets the same
     treatment in a quieter colour — also skippable, but less contentious. */
  .ep-t.filler { color: var(--danger); }
  .ep-t.recap { color: var(--text-muted); }
  .ep-tag {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-regular);
    margin-left: 6px; text-transform: lowercase;
  }
  .ep-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-top: 3px; }
  .ep-n { font-family: var(--font-display); font-size: 1.25rem; font-weight: var(--fw-semibold); color: var(--text-muted); }

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
  /* A labelled field, not an accent-tinted pill: this is a control you set
     once, not a filter you toggle, so it takes the same shape as every other
     input on the site (inset surface, hairline border, accent on focus). */
  .scope { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: var(--space-5); }
  .scope-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    text-transform: uppercase; letter-spacing: 0.1em;
  }
  .scope-select {
    min-height: 38px; padding: 0 12px; max-width: 320px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary);
    font-family: inherit; font-size: var(--fs-small); cursor: pointer; outline: none;
  }
  .scope-select:hover { border-color: var(--border-strong); }
  .scope-select:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--focus-ring); }
  .scope-select option { background: var(--surface-raised); color: var(--text-primary); }

  .sim { margin-top: var(--space-8); }
  .sim-head {
    display: flex; align-items: baseline; justify-content: space-between;
    border-bottom: 1px solid var(--border-subtle); padding-bottom: var(--space-3); margin-bottom: var(--space-5);
  }
  .sim-head h2 { font-size: var(--fs-h2); }

  /* ---- franchise grid ---- */
  /* Wide cards, not posters: the relation label ("Versiune alternativă") is
     the reason the card is there at all, and it needs a line to sit on. */
  .fran { display: grid; grid-template-columns: repeat(auto-fill, minmax(260px, 1fr)); gap: 12px; }
  .fr {
    display: flex; gap: 13px; align-items: stretch;
    border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised); overflow: hidden;
    transition: border-color var(--motion-fast) var(--ease);
  }
  .fr:hover { border-color: var(--border-default); }
  .fr:hover .fr-t { color: var(--accent); }
  .fr-art {
    flex: 0 0 62px; background-size: cover; background-position: center;
    background-color: var(--surface-overlay);
  }
  .fr-body { display: flex; flex-direction: column; gap: 4px; padding: 11px 13px 11px 0; min-width: 0; }
  .fr-kind {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent);
  }
  .fr-t {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary);
    line-height: 1.35; transition: color var(--motion-fast) var(--ease);
    display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2;
    -webkit-box-orient: vertical; overflow: hidden;
  }
  .fr-m { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  @media (max-width: 760px) {
    .hero-grid { grid-template-columns: 130px 1fr; gap: var(--space-4); }
    .syn { display: -webkit-box; -webkit-line-clamp: 4; line-clamp: 4; -webkit-box-orient: vertical; overflow: hidden; }
    .eps { grid-template-columns: minmax(0, 1fr); }
  }
  @media (max-width: 520px) {
    .hero-grid { grid-template-columns: minmax(0, 1fr); }
    .poster { max-width: 170px; }
  }
</style>
