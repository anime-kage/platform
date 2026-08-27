<script lang="ts">
  import RichText from '$lib/components/RichText.svelte';
  import PagePicker from '$lib/components/PagePicker.svelte';
  import { mediaUrl } from '$lib/media';
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { reltime } from '$lib/reltime';
  import { nameHue } from '$lib/avatar';
  import type {
    CommunityMember,
    ActivityEvent,
    ForumThread,
    HallOfFame,
    HallEntry
  } from '$shared/types';

  let { data } = $props();

  type Tab = 'recenzii' | 'activitate' | 'membri' | 'forum' | 'echipa' | 'panteon';
  const TABS: [Tab, string][] = [
    ['recenzii', 'Recenzii'],
    ['activitate', 'Activitate'],
    ['membri', 'Membri'],
    ['forum', 'Forum'],
    ['echipa', 'Echipă'],
    ['panteon', 'Hall of Fame']
  ];
  const TAB_KEYS = TABS.map((t) => t[0]);

  const urlTab = page.url.searchParams.get('tab') as Tab | null;
  let tab = $state<Tab>(urlTab && TAB_KEYS.includes(urlTab) ? urlTab : 'recenzii');

  const auth = $derived($authStore);

  // roles → Romanian label for the Echipă tab
  const ROLE_LABEL: Record<string, string> = {
    admin: 'Administrator',
    moderator: 'Moderator',
    coordinator: 'Coordonator',
    verifier: 'Verificator',
    translator: 'Traducător',
    user: 'Membru'
  };
  const roleLabel = (r: string) => ROLE_LABEL[r] ?? r;

  // Echipă tab: staff grouped by role, in seniority order, each with a plural
  // heading. No per-card role tag (the heading carries it) and no bios.
  const ROLE_ORDER = ['admin', 'moderator', 'coordinator', 'verifier', 'translator'];
  const ROLE_PLURAL: Record<string, string> = {
    admin: 'Administratori',
    moderator: 'Moderatori',
    coordinator: 'Coordonatori',
    verifier: 'Verificatori',
    translator: 'Traducători'
  };
  const teamGroups = $derived(
    ROLE_ORDER.map((role) => ({ role, members: data.team.filter((m) => m.role === role) })).filter(
      (g) => g.members.length > 0
    )
  );
  const joinYear = (iso: string) => new Date(iso).getFullYear();

  // ── follow state (real, persisted) — keyed by username ──────────────────────
  let following = $state<Record<string, boolean>>({});
  let followsHydrated = $state(false);
  $effect(() => {
    if (followsHydrated || auth.isLoading || !auth.isAuthenticated || !auth.user) return;
    followsHydrated = true;
    api
      .getFollowing(auth.user.username)
      .then((res) => {
        const mine: Record<string, boolean> = {};
        for (const f of res.data) mine[f.username] = true;
        following = { ...mine, ...following };
      })
      .catch(() => {});
  });

  async function toggleFollow(username: string) {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să urmărești membri.');
      return;
    }
    if (username === auth.user?.username) return;
    const next = !following[username];
    following = { ...following, [username]: next };
    try {
      await (next ? api.followUser(username) : api.unfollowUser(username));
    } catch {
      following = { ...following, [username]: !next };
      toast.error('Nu am putut actualiza urmărirea.');
    }
  }

  // ── lazy client data (members, activity) ────────────────────────────────────
  let members = $state<CommunityMember[]>([]);
  let membersLoaded = $state(false);
  let memberQuery = $state('');

  async function loadMembers() {
    if (membersLoaded) return;
    membersLoaded = true;
    try {
      const res = await api.getCommunityMembers();
      members = res.data;
      // seed the follow map from the server's isFollowing
      const seed: Record<string, boolean> = {};
      for (const m of res.data) if (m.isFollowing) seed[m.username] = true;
      following = { ...seed, ...following };
    } catch {
      membersLoaded = false;
    }
  }

  let activity = $state<ActivityEvent[]>([]);
  let activityFriends = $state(0);
  let activityLoaded = $state(false);

  async function loadActivity() {
    if (activityLoaded || !auth.isAuthenticated) return;
    activityLoaded = true;
    try {
      const res = await api.getCommunityActivity();
      activity = res.data;
      activityFriends = res.friends;
    } catch {
      activityLoaded = false;
    }
  }

  // members power both the Membri tab and the sidebar suggestions, so load
  // them up front rather than waiting for the tab to open
  onMount(loadMembers);

  // friends-only activity is viewer-specific — fetch it when the tab is opened
  $effect(() => {
    if (tab === 'activitate' && auth.isAuthenticated) loadActivity();
  });

  const memberResults = $derived(
    memberQuery.trim()
      ? members.filter((m) => m.username.toLowerCase().includes(memberQuery.trim().toLowerCase()))
      : members
  );

  // Members tab pagination
  const MEMBERS_PER_PAGE = 8;
  let memberPage = $state(1);
  $effect(() => {
    void memberQuery; // reset to the first page whenever the search changes
    memberPage = 1;
  });
  const memberPages = $derived(Math.max(1, Math.ceil(memberResults.length / MEMBERS_PER_PAGE)));
  const pagedMembers = $derived(
    memberResults.slice((memberPage - 1) * MEMBERS_PER_PAGE, memberPage * MEMBERS_PER_PAGE)
  );

  // people to follow: real members the viewer isn't following yet, most active
  // first (no presence tracking, so "relevant" rather than literally "online")
  const suggestions = $derived(
    members.filter((m) => m.username !== auth.user?.username && !following[m.username]).slice(0, 4)
  );
  const featured = $derived(
    members.filter((m) => m.username !== auth.user?.username && !following[m.username]).slice(0, 6)
  );

  // ── hall of fame (translators + verifiers) ──────────────────────────────────
  let hallWindow = $state<'month' | 'all'>('all');
  let hall = $state<HallOfFame | null>(null);
  let hallLoading = $state(false);
  // cache each window so toggling back is instant and doesn't re-fetch
  const hallCache: Record<string, HallOfFame> = {};

  async function loadHall(win: 'month' | 'all') {
    if (hallCache[win]) {
      hall = hallCache[win];
      return;
    }
    hallLoading = true;
    try {
      const res = await api.getHallOfFame(win);
      hallCache[win] = res;
      if (hallWindow === win) hall = res;
    } catch {
      /* leave prior data */
    } finally {
      hallLoading = false;
    }
  }

  function setHallWindow(win: 'month' | 'all') {
    hallWindow = win;
    loadHall(win);
  }

  $effect(() => {
    if (tab === 'panteon') loadHall(hallWindow);
  });

  // ── forum ───────────────────────────────────────────────────────────────────
  const FORUM_CATS = ['Toate', 'Discuții', 'Recomandări', 'Teorii', 'Meta', 'Ajutor'];
  let threads = $state<ForumThread[]>(data.threads);
  let forumCat = $state('Toate');
  let composeOpen = $state(false);
  let composeTitle = $state('');
  let composeBody = $state('');
  let composeCat = $state('Discuții');
  let posting = $state(false);

  const shownThreads = $derived(
    forumCat === 'Toate' ? threads : threads.filter((t) => t.category === forumCat)
  );

  async function submitThread() {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să deschizi un subiect.');
      return;
    }
    const title = composeTitle.trim();
    const body = composeBody.trim();
    if (title.length < 3) {
      toast.error('Titlul e prea scurt.');
      return;
    }
    if (!body) {
      toast.error('Scrie primul mesaj.');
      return;
    }
    posting = true;
    try {
      const res = await api.createForumThread({ category: composeCat, title, body });
      threads = [res.data, ...threads];
      composeTitle = '';
      composeBody = '';
      composeOpen = false;
      toast.success('Subiect publicat.');
    } catch (e) {
      toast.error((e as { error?: string })?.error ?? 'Nu am putut publica subiectul.');
    } finally {
      posting = false;
    }
  }

  const stars = (rating: number) => Array.from({ length: 5 }, (_, i) => i < rating);
  const titleName = (t: { title: string; titleRomanian?: string }) => t.titleRomanian || t.title;
  const isHot = (t: ForumThread) => t.replyCount >= 5;
</script>

<svelte:head><title>Comunitate · Anime-Kage</title></svelte:head>

<div class="container comm">
  <header class="top">
    <div>
      <p class="kick">Comunitate</p>
      <h1>Activitatea comunității</h1>
    </div>
    <span class="kicker sub">Recenzii, activitate, forum și echipa Anime-Kage</span>
  </header>

  <div class="tabs">
    {#each TABS as [key, label] (key)}
      <button class="tab" class:on={tab === key} onclick={() => (tab = key)}>{label}</button>
    {/each}
  </div>

  <div class="cols">
    <div class="main">
      {#if tab === 'recenzii'}
        {#if data.reviews.length === 0}
          <p class="none">Încă nu există recenzii. Fii primul care scrie una din pagina unui titlu.</p>
        {/if}
        {#each data.reviews as r (r.kind + r.entryId)}
          {@const href = `/${r.kind}/${r.title.id}`}
          {@const reviewHref = `/${r.kind}/${r.title.id}/review/${r.entryId}`}
          <article class="review">
            <a class="r-poster media-tone" href={href} style={r.title.imageUrl ? `background-image:url(${mediaUrl(r.title.imageUrl)})` : ''} aria-label={titleName(r.title)}></a>
            <div class="r-main">
              <div class="r-head">
                {#if r.user.avatarUrl}
                  <a class="avatar sm" href={`/user/${r.user.username}`}><img src={api.resolveUrl(r.user.avatarUrl)} alt={r.user.username} /></a>
                {:else}
                  <a class="avatar sm monogram" href={`/user/${r.user.username}`} style={`--mg-hue:${nameHue(r.user.username)}`}>{r.user.username.charAt(0).toUpperCase()}</a>
                {/if}
                <a class="r-name plink" href={`/user/${r.user.username}`}>{r.user.username}</a>
                {#if r.user.username !== auth.user?.username}
                  <button class="follow" class:on={following[r.user.username]} onclick={() => toggleFollow(r.user.username)}>
                    {following[r.user.username] ? '✓ Urmărești' : '+ Urmărește'}
                  </button>
                {/if}
                <span class="r-date">{reltime(r.updatedAt)}</span>
              </div>
              <a class="r-title" href={href}>
                <em>{titleName(r.title)}</em>
                {#if r.score}
                  <span class="stars">
                    {#each stars(Math.round(r.score / 2)) as full, si (si)}<span class:full>★</span>{/each}
                    <span class="score-n">{r.score / 2}/5</span>
                  </span>
                {/if}
              </a>
              <!-- interactive={false}: this whole row is a link, and a reveal
                   button cannot live inside one. Open the review to read it. -->
              <a class="r-text" href={reviewHref}><RichText text={r.notes} interactive={false} /></a>
              <div class="r-actions">
                <a class="ra" href={`${reviewHref}#comentarii`}><span>💬</span>{r.replyCount ? `${r.replyCount} ${r.replyCount === 1 ? 'răspuns' : 'răspunsuri'}` : 'Comentează'}</a>
                <a class="ra" href={reviewHref}>Vezi recenzia →</a>
                <span class="ra-kind">{r.kind === 'anime' ? 'Anime' : 'Manga'}</span>
              </div>
            </div>
          </article>
        {/each}

      {:else if tab === 'activitate'}
        <div class="friends-note">
          <span class="fn-ico" aria-hidden="true">👥</span>
          <span>Vezi doar activitatea <strong>prietenilor</strong> — persoanele pe care le urmărești și care te urmăresc înapoi.</span>
        </div>
        {#if !auth.isAuthenticated}
          <p class="none">Autentifică-te ca să vezi activitatea prietenilor tăi.</p>
        {:else if activityFriends === 0}
          <div class="empty">
            <span class="empty-ico" aria-hidden="true">👋</span>
            <p class="empty-title">Niciun prieten încă</p>
            <p class="empty-sub">Prietenii sunt urmăririle reciproce. Urmărește membri din fila <button class="linklike" onclick={() => (tab = 'membri')}>Membri</button> — când te urmăresc și ei înapoi, activitatea lor apare aici.</p>
          </div>
        {:else if activity.length === 0}
          <p class="none">Prietenii tăi n-au activitate recentă.</p>
        {:else}
          {#each activity as ev (ev.type + ev.user.id + ev.target + ev.createdAt)}
            <div class="act">
              {#if ev.user.avatarUrl}
                <a class="avatar" href={`/user/${ev.user.username}`}><img src={api.resolveUrl(ev.user.avatarUrl)} alt={ev.user.username} /></a>
              {:else}
                <a class="avatar monogram" href={`/user/${ev.user.username}`} style={`--mg-hue:${nameHue(ev.user.username)}`}>{ev.user.username.charAt(0).toUpperCase()}</a>
              {/if}
              <span class="act-body">
                <a class="plink" href={`/user/${ev.user.username}`}><strong>{ev.user.username}</strong></a>
                {ev.verb}
                {#if ev.link}<a class="act-tgt" href={ev.link}>{ev.target}</a>{:else}<em class="act-tgt">{ev.target}</em>{/if}
                {#if ev.meta}<span class="act-meta">{ev.meta}</span>{/if}
              </span>
              <span class="act-time">{reltime(ev.createdAt)}</span>
            </div>
          {/each}
        {/if}

      {:else if tab === 'membri'}
        {#if featured.length > 0 && !memberQuery.trim()}
          <div class="featured">
            <p class="featured-h">Recomandați pentru tine</p>
            <div class="featured-row">
              {#each featured as m (m.id)}
                <div class="fcard">
                  {#if m.avatarUrl}
                    <a class="avatar lg" href={`/user/${m.username}`}><img src={api.resolveUrl(m.avatarUrl)} alt={m.username} /></a>
                  {:else}
                    <a class="avatar lg monogram" href={`/user/${m.username}`} style={`--mg-hue:${nameHue(m.username)}`}>{m.username.charAt(0).toUpperCase()}</a>
                  {/if}
                  <a class="fcard-name plink" href={`/user/${m.username}`}>{m.username}</a>
                  <span class="fcard-meta">{m.role !== 'user' ? roleLabel(m.role) : `${m.reviewCount} recenzii`}</span>
                  <button class="follow" class:on={following[m.username]} onclick={() => toggleFollow(m.username)}>
                    {following[m.username] ? '✓ Urmărești' : '+ Urmărește'}
                  </button>
                </div>
              {/each}
            </div>
          </div>
        {/if}

        <div class="msearch">
          <span class="msearch-ico">⌕</span>
          <input bind:value={memberQuery} placeholder="Caută un membru după nume..." />
        </div>
        {#if !membersLoaded && members.length === 0}
          <p class="none">Se încarcă membrii…</p>
        {:else if memberResults.length === 0}
          <p class="none">Niciun membru găsit.</p>
        {:else}
          {#each pagedMembers as m (m.id)}
            <div class="member">
              {#if m.avatarUrl}
                <a class="avatar lg" href={`/user/${m.username}`}><img src={api.resolveUrl(m.avatarUrl)} alt={m.username} /></a>
              {:else}
                <a class="avatar lg monogram" href={`/user/${m.username}`} style={`--mg-hue:${nameHue(m.username)}`}>{m.username.charAt(0).toUpperCase()}</a>
              {/if}
              <div class="member-main">
                <div class="member-head">
                  <a class="member-name plink" href={`/user/${m.username}`}>{m.username}</a>
                  {#if m.role !== 'user'}<span class="role-tag">{roleLabel(m.role)}</span>{/if}
                  {#if m.username !== auth.user?.username}
                    <button class="follow" class:on={following[m.username]} onclick={() => toggleFollow(m.username)}>
                      {following[m.username] ? '✓ Urmărești' : '+ Urmărește'}
                    </button>
                  {/if}
                  <span class="member-stats"><strong>{m.followers}</strong> urm. · <strong>{m.reviewCount}</strong> rec. · <strong>{m.listCount}</strong> liste</span>
                </div>
                {#if m.bio}<p class="member-bio">{m.bio}</p>{/if}
              </div>
            </div>
          {/each}

          {#if memberPages > 1}
            <div class="pager">
              <button class="pg-btn" disabled={memberPage === 1} onclick={() => (memberPage = Math.max(1, memberPage - 1))}>← Anterior</button>
              <PagePicker page={memberPage} pages={memberPages} onselect={(n) => (memberPage = n)} />
              <button class="pg-btn" disabled={memberPage === memberPages} onclick={() => (memberPage = Math.min(memberPages, memberPage + 1))}>Următor →</button>
            </div>
          {/if}
        {/if}

      {:else if tab === 'echipa'}
        <p class="team-intro">Oamenii care traduc, verifică și coordonează conținutul de pe Anime-Kage.</p>
        {#each teamGroups as g (g.role)}
          <section class="team-group">
            <header class="tg-head">
              <h2 class="tg-title">{ROLE_PLURAL[g.role]}</h2>
              <span class="tg-count">{g.members.length}</span>
            </header>
            <div class="tg-people">
              {#each g.members as m (m.id)}
                <a class="tperson" href={`/user/${m.username}`}>
                  {#if m.avatarUrl}
                    <span class="avatar lg"><img src={api.resolveUrl(m.avatarUrl)} alt={m.username} /></span>
                  {:else}
                    <span class="avatar lg monogram" style={`--mg-hue:${nameHue(m.username)}`}>{m.username.charAt(0).toUpperCase()}</span>
                  {/if}
                  <span class="tperson-main">
                    <span class="tperson-name">{m.username}</span>
                    <span class="tperson-since">din {joinYear(m.createdAt)}</span>
                  </span>
                </a>
              {/each}
            </div>
          </section>
        {/each}

      {:else if tab === 'panteon'}
        <div class="hof-intro">
          <p class="hof-lead">Cei care duc muncă la capăt. <strong>Traducătorii</strong> urcă pentru fiecare release publicat; <strong>verificatorii</strong>, pentru fiecare release trecut prin verificare.</p>
          <div class="wtabs" role="tablist" aria-label="Interval clasament">
            <button class="wtab" class:on={hallWindow === 'month'} role="tab" aria-selected={hallWindow === 'month'} onclick={() => setHallWindow('month')}>Luna aceasta</button>
            <button class="wtab" class:on={hallWindow === 'all'} role="tab" aria-selected={hallWindow === 'all'} onclick={() => setHallWindow('all')}>Din totdeauna</button>
          </div>
        </div>

        {#snippet board(title: string, unit: string, entries: HallEntry[])}
          <section class="hof-panel">
            <header class="hof-panel-head">
              <span class="hof-panel-title">{title}</span>
              <span class="kicker">{unit}</span>
            </header>
            {#if entries.length === 0}
              <p class="hof-empty">Niciun release {title === 'Traducători' ? 'tradus' : 'verificat'} {hallWindow === 'month' ? 'luna aceasta' : 'încă'}.</p>
            {:else}
              {#each entries as e, i (e.userId)}
                <a class="hrank" class:top={i < 3} href={`/user/${e.username}`}>
                  <span class="hrank-n">{i + 1}</span>
                  {#if e.avatarUrl}
                    <span class="avatar"><img src={api.resolveUrl(e.avatarUrl)} alt={e.username} /></span>
                  {:else}
                    <span class="avatar monogram" style={`--mg-hue:${nameHue(e.username)}`}>{e.username.charAt(0).toUpperCase()}</span>
                  {/if}
                  <span class="hrank-main">
                    <span class="hrank-name">{e.username}</span>
                    {#if e.role !== 'user'}<span class="hrank-role">{roleLabel(e.role)}</span>{/if}
                  </span>
                  <span class="hrank-s">{e.count}</span>
                </a>
              {/each}
            {/if}
          </section>
        {/snippet}

        {#if !hall && hallLoading}
          <p class="none">Se încarcă clasamentul…</p>
        {:else if hall}
          <div class="boards" class:dim={hallLoading}>
            {@render board('Traducători', 'traduceri', hall.translators)}
            {@render board('Verificatori', 'verificări', hall.verifiers)}
          </div>
        {/if}

      {:else}
        <!-- FORUM -->
        <div class="forum-bar">
          <div class="fcats">
            {#each FORUM_CATS as c (c)}
              <button class="chip" class:on={forumCat === c} onclick={() => (forumCat = c)}>{c}</button>
            {/each}
          </div>
          <button class="btn fill sm" onclick={() => (composeOpen = !composeOpen)}>+ Subiect nou</button>
        </div>

        {#if composeOpen}
          <div class="compose anim-fade">
            <p class="compose-t">Deschide un subiect nou</p>
            <input class="compose-title" bind:value={composeTitle} placeholder="Titlul subiectului" maxlength="160" />
            <textarea bind:value={composeBody} placeholder="Scrie primul mesaj..." maxlength="8000"></textarea>
            <div class="compose-foot">
              <span class="kicker">Categorie</span>
              <select bind:value={composeCat}>
                {#each FORUM_CATS.slice(1) as c (c)}<option value={c}>{c}</option>{/each}
              </select>
              <span class="spacer"></span>
              <button class="btn ghost sm" onclick={() => (composeOpen = false)}>Anulează</button>
              <button class="btn fill sm" onclick={submitThread} disabled={posting}>{posting ? 'Se publică…' : 'Publică subiectul'}</button>
            </div>
          </div>
        {/if}

        {#if shownThreads.length === 0}
          <p class="none">Niciun subiect {forumCat !== 'Toate' ? `în „${forumCat}”` : 'încă'}. Deschide tu primul.</p>
        {/if}
        {#each shownThreads as t (t.id)}
          <a class="thread" href={`/comunitate/forum/${t.id}`}>
            {#if t.author.avatarUrl}
              <span class="avatar sq"><img src={api.resolveUrl(t.author.avatarUrl)} alt={t.author.username} /></span>
            {:else}
              <span class="avatar sq monogram" style={`--mg-hue:${nameHue(t.author.username)}`}>{t.author.username.charAt(0).toUpperCase()}</span>
            {/if}
            <div class="thread-main">
              <div class="thread-head">
                {#if t.isPinned}<span class="pin" title="Fixat">★</span>{/if}
                {#if t.isLocked}<span class="lock" title="Blocat">🔒</span>{/if}
                <span class="thread-t">{t.title}</span>
                {#if isHot(t)}<span class="hot">hot</span>{/if}
              </div>
              <div class="thread-m">
                <span class="thread-cat">{t.category}</span>
                <span>·</span>
                <span>de {t.author.username}</span>
                <span>·</span>
                <span>activ {reltime(t.lastActivityAt)}</span>
              </div>
            </div>
            <div class="thread-r">
              <span class="thread-count">{t.replyCount}</span>
              <span class="thread-lbl">răsp.</span>
            </div>
          </a>
        {/each}
      {/if}
    </div>

    <aside class="side">
      <div>
        <p class="sect kicker">Utilizatori recomandați</p>
        {#if suggestions.length === 0}
          <p class="side-empty">{membersLoaded ? 'Îi urmărești deja pe toți recomandații 🎉' : 'Se încarcă…'}</p>
        {:else}
          {#each suggestions as m (m.id)}
            <div class="orow">
              {#if m.avatarUrl}
                <a class="avatar" href={`/user/${m.username}`}><img src={api.resolveUrl(m.avatarUrl)} alt={m.username} /></a>
              {:else}
                <a class="avatar monogram" href={`/user/${m.username}`} style={`--mg-hue:${nameHue(m.username)}`}>{m.username.charAt(0).toUpperCase()}</a>
              {/if}
              <span class="orow-main">
                <a class="orow-name plink" href={`/user/${m.username}`}>{m.username}</a>
                <span class="orow-status">{m.role !== 'user' ? roleLabel(m.role) : `${m.followers} urmăritori`}</span>
              </span>
              <button class="follow" class:on={following[m.username]} onclick={() => toggleFollow(m.username)}>
                {following[m.username] ? '✓' : '+'}
              </button>
            </div>
          {/each}
        {/if}
      </div>

      <div>
        <p class="sect kicker">Echipa</p>
        {#each data.team.slice(0, 5) as m (m.id)}
          <div class="orow tight">
            {#if m.avatarUrl}
              <a class="avatar xs2" href={`/user/${m.username}`}><img src={api.resolveUrl(m.avatarUrl)} alt={m.username} /></a>
            {:else}
              <a class="avatar xs2 monogram" href={`/user/${m.username}`} style={`--mg-hue:${nameHue(m.username)}`}>{m.username.charAt(0).toUpperCase()}</a>
            {/if}
            <span class="orow-main">
              <a class="orow-name sm plink" href={`/user/${m.username}`}>{m.username}</a>
              <span class="orow-status">{roleLabel(m.role)}</span>
            </span>
          </div>
        {/each}
        <button class="see-all" onclick={() => (tab = 'echipa')}>Vezi toată echipa →</button>
      </div>

      <div class="stats-card">
        <div class="stat">
          <span class="stat-v">{data.stats.members}</span>
          <span class="stat-l kicker">Membri</span>
        </div>
        <div class="stat">
          <span class="stat-v">{data.stats.titles}</span>
          <span class="stat-l kicker">Titluri</span>
        </div>
        <!-- Online, not published subtitles: the catalog is mostly imported,
             so a subtitle count read as a number nobody could interpret. -->
        <div class="stat">
          <span class="stat-v">{data.stats.online ?? 0}</span>
          <span class="stat-l kicker">Membri online</span>
        </div>
      </div>
    </aside>
  </div>
</div>

<style>
  .comm { padding-block: var(--space-6) var(--space-8); max-width: 1180px; }

  .plink { color: inherit; }
  .plink:hover { color: var(--accent); }

  .top {
    display: flex; align-items: flex-end; justify-content: space-between;
    flex-wrap: wrap; gap: var(--space-4);
    padding-bottom: 18px; border-bottom: 2px solid var(--text-primary);
  }
  .kick { font-size: var(--fs-caption); font-weight: var(--fw-bold); color: var(--accent); }
  .top h1 { font-family: var(--font-display); font-size: clamp(2rem, 1.6rem + 2vw, 2.625rem); letter-spacing: -0.02em; line-height: 1; margin-top: 10px; }
  .sub { color: var(--text-muted); }

  .tabs { display: flex; gap: 6px; margin: 18px 0 28px; flex-wrap: wrap; }
  .tab {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    padding: 7px 14px; border-radius: var(--radius-pill); border: 1px solid transparent;
    background: none; cursor: pointer;
  }
  .tab:hover { color: var(--text-primary); }
  .tab.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }

  .cols { display: grid; grid-template-columns: minmax(0, 1fr) 296px; gap: 40px; align-items: start; }
  .main { min-width: 0; }

  /* ---- avatars ---- */
  .avatar {
    width: 34px; height: 34px; border-radius: 50%; flex: 0 0 auto;
    display: grid; place-items: center; overflow: hidden;
    font-size: 0.8125rem; font-weight: var(--fw-bold); color: #fff;
  }
  .avatar img { width: 100%; height: 100%; object-fit: cover; }
  .avatar.sm { width: 26px; height: 26px; font-size: var(--fs-micro); }
  .avatar.xs2 { width: 32px; height: 32px; font-size: 0.75rem; }
  .avatar.lg { width: 46px; height: 46px; font-size: 1.0625rem; }
  .avatar.sq { width: 38px; height: 38px; border-radius: 10px; font-size: 0.875rem; }

  .follow {
    font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    padding: 4px 10px; border-radius: var(--radius-pill);
    border: 1px solid var(--border-default); background: none;
    color: var(--text-muted); cursor: pointer; white-space: nowrap;
  }
  .follow:hover { color: var(--text-primary); }
  .follow.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }

  .role-tag {
    font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    color: var(--accent); padding: 2px 8px; border-radius: var(--radius-pill);
    border: 1px solid color-mix(in srgb, var(--accent) 45%, transparent);
    white-space: nowrap;
  }
  .role-tag.on { background: color-mix(in srgb, var(--accent) 12%, transparent); }

  /* ---- reviews ---- */
  .review { display: flex; gap: 18px; padding: 24px 0; border-bottom: 1px solid var(--border-subtle); }
  .r-poster {
    width: 62px; height: 93px; border-radius: 6px; flex: 0 0 auto;
    background-color: var(--surface-overlay);
    background-size: cover; background-position: center;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }
  .r-main { flex: 1; min-width: 0; }
  .r-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .r-name { font-size: var(--fs-small); font-weight: var(--fw-semibold); }
  .r-date { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-left: auto; white-space: nowrap; }
  .r-title { display: flex; align-items: baseline; gap: 10px; margin: 11px 0 9px; flex-wrap: wrap; }
  .r-title em {
    font-family: var(--font-display); font-size: 1.125rem;
    font-weight: var(--fw-semibold); font-style: italic; color: var(--text-primary);
  }
  .r-title:hover em { color: var(--accent); }
  .stars { display: inline-flex; align-items: center; gap: 3px; letter-spacing: 1.5px; font-size: 0.8125rem; color: var(--surface-overlay); }
  .stars .full { color: var(--accent); }
  .score-n { font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0; color: var(--text-muted); margin-left: 5px; }
  .r-text { display: block; font-size: var(--fs-small); line-height: 1.6; color: var(--text-muted); }
  .r-text:hover { color: var(--text-primary); }
  .r-actions { display: flex; align-items: center; gap: 18px; margin-top: 14px; flex-wrap: wrap; }
  .ra { display: flex; align-items: center; gap: 6px; background: none; border: none; font-family: var(--font-mono); font-size: 0.75rem; color: var(--text-muted); }
  .ra:hover { color: var(--accent); }
  .ra-kind {
    font-family: var(--font-mono); font-size: var(--fs-micro); text-transform: uppercase; letter-spacing: 0.05em;
    color: var(--text-faint); border: 1px solid var(--border-subtle); padding: 1px 7px; border-radius: 5px;
  }

  /* ---- activity ---- */
  .friends-note {
    display: flex; align-items: center; gap: 10px; margin-bottom: 20px;
    padding: 12px 15px; border-radius: var(--radius-md);
    background: color-mix(in srgb, var(--accent) 7%, transparent);
    border: 1px solid color-mix(in srgb, var(--accent) 25%, transparent);
    font-size: var(--fs-caption); color: var(--text-muted); line-height: 1.4;
  }
  .friends-note strong { color: var(--text-primary); }
  .fn-ico { font-size: 1.1rem; }
  .act { display: flex; align-items: center; gap: 13px; padding: 15px 0; border-bottom: 1px solid var(--border-subtle); }
  .act-body { flex: 1; min-width: 0; font-size: var(--fs-small); line-height: 1.5; color: var(--text-muted); }
  .act-body strong { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .act-tgt { font-family: var(--font-display); font-style: italic; color: var(--text-primary); }
  a.act-tgt:hover { color: var(--accent); }
  .act-meta { color: var(--accent); font-family: var(--font-mono); font-size: 0.75rem; margin-left: 4px; }
  .act-time { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); white-space: nowrap; }

  /* ---- members ---- */
  .msearch {
    display: flex; align-items: center; gap: 10px;
    margin-bottom: 22px; padding: 11px 15px;
    border: 1px solid var(--border-default); border-radius: 11px; background: var(--surface-raised);
  }
  .msearch:focus-within { border-color: var(--accent); }
  .msearch-ico { color: var(--text-muted); font-size: 0.9375rem; }
  .msearch input { flex: 1; min-width: 0; background: none; border: none; outline: none; color: var(--text-primary); font-size: var(--fs-small); }
  .msearch input::placeholder { color: var(--text-faint); }

  .member { display: flex; align-items: flex-start; gap: 13px; padding: 17px 0; border-bottom: 1px solid var(--border-subtle); }
  .member-main { flex: 1; min-width: 0; }
  .member-head { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .member-name { font-family: var(--font-display); font-size: 1.03125rem; font-weight: var(--fw-semibold); }
  .member-stats { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-left: auto; white-space: nowrap; }
  .member-stats strong { color: var(--text-primary); font-weight: var(--fw-medium); }
  .member-bio { font-size: var(--fs-caption); line-height: 1.5; color: var(--text-muted); margin-top: 4px; max-width: 560px; }
  .none { color: var(--text-muted); padding: 20px 0; font-size: var(--fs-small); }

  /* featured recommendations strip (top of Members tab) */
  .featured { margin-bottom: 24px; }
  .featured-h {
    font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.08em;
    text-transform: uppercase; color: var(--accent); margin-bottom: 12px;
  }
  .featured-row {
    display: grid; grid-auto-flow: column; grid-auto-columns: minmax(150px, 1fr);
    gap: 12px; overflow-x: auto; padding-bottom: 6px;
  }
  .fcard {
    display: flex; flex-direction: column; align-items: center; gap: 7px; text-align: center;
    padding: 16px 12px; border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised);
  }
  .fcard .avatar.lg { width: 52px; height: 52px; font-size: 1.25rem; }
  .fcard-name { font-weight: var(--fw-semibold); font-size: var(--fs-small); color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
  .fcard-meta { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .fcard .follow { margin-top: 3px; }

  /* pagination */
  .pager { display: flex; align-items: center; justify-content: center; gap: 16px; margin-top: 24px; }
  .pg-btn {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    background: none; border: 1px solid var(--border-default); border-radius: 8px;
    padding: 8px 14px; cursor: pointer;
  }
  .pg-btn:hover:not(:disabled) { color: var(--text-primary); border-color: var(--accent); }
  .pg-btn:disabled { opacity: 0.4; cursor: default; }
  .pg-info { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  /* ---- team (grouped by role) ---- */
  .team-intro { color: var(--text-muted); font-size: var(--fs-small); margin-bottom: 26px; }
  .team-group { margin-bottom: 30px; }
  .tg-head { display: flex; align-items: center; gap: 10px; margin-bottom: 16px; }
  .tg-title { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .tg-count {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    background: var(--surface-overlay); padding: 1px 8px; border-radius: var(--radius-pill);
  }
  .tg-head::after { content: ''; flex: 1; height: 1px; background: var(--border-subtle); }
  .tg-people { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, 190px), 1fr)); gap: 10px; }
  .tperson {
    display: flex; align-items: center; gap: 12px; padding: 10px 12px;
    border: 1px solid var(--border-subtle); border-radius: var(--radius-md);
    background: var(--surface-raised); color: inherit; min-width: 0;
  }
  .tperson:hover { border-color: var(--border-default); background: var(--surface-overlay); }
  .tperson-main { display: flex; flex-direction: column; min-width: 0; }
  .tperson-name { font-weight: var(--fw-semibold); font-size: var(--fs-small); color: var(--text-primary); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .tperson:hover .tperson-name { color: var(--accent); }
  .tperson-since { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  /* ---- hall of fame ---- */
  .hof-intro {
    display: flex; align-items: flex-end; justify-content: space-between;
    gap: 20px 32px; flex-wrap: wrap; margin-bottom: 26px;
  }
  .hof-lead { color: var(--text-muted); font-size: var(--fs-small); line-height: 1.55; max-width: 540px; }
  .hof-lead strong { color: var(--text-primary); font-weight: var(--fw-semibold); }
  .wtabs { display: flex; border-bottom: 1px solid var(--border-subtle); flex: 0 0 auto; }
  .wtab {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold); color: var(--text-muted);
    background: none; border: none; cursor: pointer; padding: 9px 16px;
    border-bottom: 2px solid transparent; margin-bottom: -1px; white-space: nowrap;
    transition: color var(--motion-fast) var(--ease);
  }
  .wtab:hover { color: var(--text-primary); }
  .wtab.on { color: var(--text-primary); border-bottom-color: var(--accent); }

  .boards { display: grid; grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); gap: 20px; transition: opacity 0.15s; }
  .boards.dim { opacity: 0.5; }
  .hof-panel {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }
  .hof-panel-head {
    display: flex; align-items: baseline; justify-content: space-between;
    padding: 14px 16px; border-bottom: 1px solid var(--border-subtle);
  }
  .hof-panel-title { font-family: var(--font-display); font-size: var(--fs-body); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .hof-empty { padding: 24px 16px; font-size: var(--fs-small); color: var(--text-muted); text-align: center; }

  .hrank {
    display: flex; align-items: center; gap: 13px;
    padding: 11px 16px; border-bottom: 1px solid var(--border-subtle); color: inherit;
  }
  .hrank:last-child { border-bottom: none; }
  .hrank:hover { background: var(--surface-overlay); }
  .hrank-n {
    font-family: var(--font-display); font-size: 1.0625rem; font-weight: var(--fw-semibold);
    color: var(--text-faint); width: 20px; text-align: center; flex: 0 0 auto; font-variant-numeric: tabular-nums;
  }
  .hrank.top .hrank-n { color: var(--accent); }
  .hrank .avatar { width: 36px; height: 36px; font-size: 0.9375rem; }
  .hrank-main { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 2px; }
  .hrank-name {
    font-size: var(--fs-small); font-weight: var(--fw-semibold); color: var(--text-primary);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .hrank:hover .hrank-name { color: var(--accent); }
  .hrank-role { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .hrank-s {
    font-family: var(--font-display); font-size: 1.1875rem; font-weight: var(--fw-bold);
    color: var(--accent); flex: 0 0 auto; font-variant-numeric: tabular-nums;
  }
  .hrank:not(.top) .hrank-s { color: var(--text-primary); }

  /* ---- forum ---- */
  .forum-bar { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 18px; flex-wrap: wrap; }
  .fcats { display: flex; gap: 8px; flex-wrap: wrap; }
  .chip {
    font-size: var(--fs-caption); font-weight: var(--fw-medium); color: var(--text-muted);
    padding: 5px 12px; border: 1px solid var(--border-subtle); border-radius: var(--radius-pill);
    background: none; cursor: pointer;
  }
  .chip:hover { color: var(--text-primary); }
  .chip.on { color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent); background: color-mix(in srgb, var(--accent) 10%, transparent); }

  .btn { font-weight: var(--fw-semibold); font-size: 0.8125rem; padding: 9px 16px; border-radius: 9px; cursor: pointer; white-space: nowrap; }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.fill:disabled { opacity: 0.6; cursor: default; }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-muted); }
  .btn.ghost:hover { color: var(--text-primary); }
  .btn.sm { font-size: 0.78125rem; padding: 8px 14px; }

  .compose { border: 1px solid var(--accent); border-radius: var(--radius-lg); background: var(--surface-raised); padding: 20px; margin-bottom: 24px; }
  .compose-t { font-family: var(--font-display); font-size: 1.0625rem; font-weight: var(--fw-semibold); margin-bottom: 15px; }
  .compose input.compose-title {
    width: 100%; margin-bottom: 11px;
    background: var(--surface-overlay); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); padding: 12px 14px; color: var(--text-primary); outline: none;
    font-size: var(--fs-body); font-weight: var(--fw-semibold);
  }
  .compose textarea {
    width: 100%; min-height: 96px; resize: vertical;
    background: var(--surface-overlay); border: 1px solid var(--border-subtle);
    border-radius: var(--radius-md); padding: 12px 14px; color: var(--text-primary); outline: none;
    font-size: var(--fs-small); line-height: 1.5;
  }
  .compose input:focus, .compose textarea:focus { border-color: var(--accent); }
  .compose-foot { display: flex; align-items: center; gap: 12px; margin-top: 13px; flex-wrap: wrap; }
  .compose-foot select { background: var(--surface-overlay); border: 1px solid var(--border-default); border-radius: 8px; padding: 8px 12px; color: var(--text-primary); outline: none; font-size: 0.8125rem; cursor: pointer; }
  .spacer { flex: 1; }

  .thread { display: flex; gap: 15px; align-items: flex-start; padding: 17px 0; border-bottom: 1px solid var(--border-subtle); color: inherit; }
  .thread:hover .thread-t { color: var(--accent); }
  .thread-main { flex: 1; min-width: 0; }
  .thread-head { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .pin { font-size: 0.75rem; color: var(--accent); }
  .lock { font-size: 0.7rem; }
  .thread-t { font-family: var(--font-display); font-size: 1.03125rem; font-weight: var(--fw-semibold); line-height: 1.25; }
  .hot {
    font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.06em; text-transform: uppercase;
    color: var(--on-accent); background: var(--accent); padding: 2px 6px; border-radius: 5px;
  }
  .thread-m { display: flex; align-items: center; gap: 8px; margin-top: 8px; flex-wrap: wrap; font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }
  .thread-cat { color: var(--accent); }
  .thread-r { display: flex; flex-direction: column; align-items: flex-end; gap: 2px; flex: 0 0 auto; }
  .thread-count { font-family: var(--font-display); font-size: 1.1875rem; font-weight: var(--fw-semibold); }
  .thread-lbl { font-family: var(--font-mono); font-size: var(--fs-micro); letter-spacing: 0.05em; text-transform: uppercase; color: var(--text-muted); }

  /* ---- empty states ---- */
  .empty { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 40px 20px; text-align: center; }
  .empty-ico { font-size: 2rem; opacity: 0.5; }
  .empty-title { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .empty-sub { font-size: var(--fs-caption); color: var(--text-muted); max-width: 380px; line-height: 1.5; }
  .linklike { background: none; border: none; color: var(--accent); cursor: pointer; font: inherit; padding: 0; }
  .linklike:hover { text-decoration: underline; }

  /* ---- shared side ---- */
  .sect { display: block; padding-bottom: 12px; border-bottom: 1px solid var(--border-default); margin-bottom: 6px; }
  .side-empty { font-size: var(--fs-caption); color: var(--text-muted); padding: 8px 0; }
  .orow { display: flex; align-items: center; gap: 12px; padding: 12px 0; border-bottom: 1px solid var(--border-subtle); }
  .orow.tight { padding: 9px 0; border-bottom: none; }
  .orow-main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .orow-name { font-size: var(--fs-small); font-weight: var(--fw-semibold); }
  .orow-name.sm { font-size: 0.8125rem; }
  .orow-status { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); }

  .see-all { margin-top: 10px; background: none; border: none; color: var(--accent); font-size: var(--fs-caption); font-weight: var(--fw-semibold); cursor: pointer; padding: 0; }
  .see-all:hover { color: var(--accent-hover); }

  .side { display: flex; flex-direction: column; gap: 26px; position: sticky; top: 80px; }
  .stats-card { display: flex; border: 1px solid var(--border-subtle); border-radius: var(--radius-lg); overflow: hidden; background: var(--surface-raised); }
  .stat { flex: 1; padding: 16px 6px; text-align: center; min-width: 0; }
  .stat + .stat { border-left: 1px solid var(--border-subtle); }
  .stat-v { display: block; font-family: var(--font-display); font-size: 1.375rem; font-weight: var(--fw-bold); color: var(--accent); }
  .stat-l { display: block; margin-top: 4px; font-size: var(--fs-micro); }

  @media (max-width: 960px) {
    .cols { grid-template-columns: minmax(0, 1fr); }
    .side { position: static; }
  }
</style>
