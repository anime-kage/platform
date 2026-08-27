<script lang="ts">
  import api from '$lib/api';
  import { authStore } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import { MEMBERS, seedNetwork } from '$lib/data/community';
  import type { FollowUser } from '$shared/types';

  let {
    kind,
    handle,
    name,
    isReal,
    rows: initialRows
  }: {
    kind: 'followers' | 'following';
    handle: string;
    name: string;
    isReal: boolean;
    rows: FollowUser[];
  } = $props();

  const auth = $derived($authStore);

  /* real accounts: SSR ships the guest view (rendered immediately);
     client-side we re-fetch with the viewer's token so each row's
     follow button reflects reality */
  let rows = $state<FollowUser[]>(initialRows);
  let hydratedFor = $state('');
  $effect(() => {
    const key = `${handle}:${kind}:${auth.isAuthenticated}`;
    if (hydratedFor === key) return;
    hydratedFor = key;
    rows = initialRows;
    if (isReal && auth.isAuthenticated) {
      (kind === 'followers' ? api.getFollowers(handle) : api.getFollowing(handle))
        .then((res) => (rows = res.data))
        .catch(() => {});
    }
  });

  // seeded demo members keep their fake network (they're not real accounts)
  const seedRows = $derived(isReal ? [] : seedNetwork(handle.toLowerCase(), kind));
  let seedFollowed = $state<Record<string, boolean>>({});

  /* community-member accounts keep their seeded look (hue avatar, display name) */
  const memberOf = (username: string) => MEMBERS.find((m) => m.id === username.toLowerCase());

  const title = $derived(kind === 'followers' ? `Urmăritorii lui ${name}` : `Urmăriți de ${name}`);
  const count = $derived(isReal ? rows.length : seedRows.length);

  let busy = $state<Record<string, boolean>>({});

  async function toggle(row: FollowUser) {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să urmărești membri.');
      return;
    }
    busy = { ...busy, [row.username]: true };
    try {
      const res = row.isFollowing
        ? await api.unfollowUser(row.username)
        : await api.followUser(row.username);
      rows = rows.map((r) =>
        r.username === row.username
          ? { ...r, isFollowing: res.data.isFollowing, followersCount: res.data.followers }
          : r
      );
    } catch {
      toast.error('Nu am putut actualiza urmărirea.');
    } finally {
      busy = { ...busy, [row.username]: false };
    }
  }

  function toggleSeed(id: string, memberName: string) {
    if (!auth.isAuthenticated) {
      toast.info('Autentifică-te ca să urmărești membri.');
      return;
    }
    seedFollowed = { ...seedFollowed, [id]: !seedFollowed[id] };
    toast.success(seedFollowed[id] ? `Îl urmărești acum pe ${memberName}.` : `Nu îl mai urmărești pe ${memberName}.`);
  }
</script>

<svelte:head><title>{title} · Anime-Kage</title></svelte:head>

<div class="container net">
  <header class="top">
    <div>
      <p class="crumb"><a href={`/user/${handle}`}>← {name}</a></p>
      <h1>{title}</h1>
    </div>
    <span class="count">{count} membri</span>
  </header>

  {#if isReal}
    {#if rows.length}
      <div class="rows">
        {#each rows as r (r.username)}
          {@const m = memberOf(r.username)}
          <div class="row">
            {#if r.avatarUrl}
              <a href={`/user/${r.username}`}><img class="ava-img" src={api.resolveUrl(r.avatarUrl)} alt={r.username} /></a>
            {:else if m}
              <a class="ava monogram" href={`/user/${r.username}`} style={`--mg-hue:${m.hue}`}>{m.name.charAt(0)}</a>
            {:else}
              <a class="ava" href={`/user/${r.username}`}>{r.username.charAt(0).toUpperCase()}</a>
            {/if}
            <a class="main" href={`/user/${r.username}`}>
              <span class="name">{m?.name ?? r.username}{#if r.role !== 'user'}<span class="role">{r.role}</span>{/if}</span>
              {#if r.bio}<span class="rbio">{r.bio}</span>{/if}
            </a>
            <span class="meta">{r.followersCount} urmăritori</span>
            {#if auth.user?.username !== r.username}
              <button
                class="fbtn"
                class:on={r.isFollowing}
                onclick={() => toggle(r)}
                disabled={busy[r.username]}
              >{r.isFollowing ? '✓ Urmărit' : '+ Urmărește'}</button>
            {/if}
          </div>
        {/each}
      </div>
    {:else}
      <p class="empty">
        {kind === 'followers' ? 'Niciun urmăritor încă.' : 'Nu urmărește pe nimeni încă.'}
        <a class="inline-link" href="/comunitate">Descoperă membri →</a>
      </p>
    {/if}
  {:else}
    <div class="rows">
      {#each seedRows as m (m.id)}
        <div class="row">
          <a class="ava monogram" href={`/user/${m.id}`} style={`--mg-hue:${m.hue}`}>{m.name.charAt(0)}</a>
          <a class="main" href={`/user/${m.id}`}>
            <span class="name">{m.name}</span>
            <span class="rbio">{m.bio}</span>
          </a>
          <span class="meta">{m.followers} urmăritori</span>
          <button class="fbtn" class:on={seedFollowed[m.id]} onclick={() => toggleSeed(m.id, m.name)}>
            {seedFollowed[m.id] ? '✓ Urmărit' : '+ Urmărește'}
          </button>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .net { max-width: 720px; padding-block: var(--space-6) var(--space-8); }

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

  /* flat hairline rows — no container box */
  .rows { display: flex; flex-direction: column; }
  .row {
    display: flex; align-items: center; gap: var(--space-4);
    padding: 16px 0; border-bottom: 1px solid var(--border-subtle);
  }
  .ava, .ava-img {
    width: 46px; height: 46px; border-radius: 50%; flex: 0 0 auto;
    border: 1px solid var(--border-default);
  }
  .ava {
    display: grid; place-items: center;
    background: linear-gradient(135deg, var(--accent), var(--accent-strong));
    font-family: var(--font-display); font-weight: var(--fw-semibold); font-size: 1.125rem; color: #fff;
  }
  .ava-img { object-fit: cover; display: block; }
  .main { flex: 1; min-width: 0; display: flex; flex-direction: column; }
  .name { font-family: var(--font-display); font-size: var(--fs-h3); font-weight: var(--fw-semibold); color: var(--text-primary); }
  .main:hover .name { color: var(--accent); }
  .role {
    margin-left: 10px; vertical-align: 2px;
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.08em; text-transform: uppercase;
    padding: 1px 7px; border: 1px solid color-mix(in srgb, var(--accent) 40%, transparent);
    border-radius: var(--radius-pill); color: var(--accent);
  }
  .rbio {
    font-size: var(--fs-caption); color: var(--text-muted); margin-top: 3px;
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis;
  }
  .meta { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); white-space: nowrap; }

  .fbtn {
    font-size: var(--fs-caption); font-weight: var(--fw-semibold);
    padding: 7px 14px; border-radius: var(--radius-pill);
    border: 1px solid var(--border-default); background: none;
    color: var(--text-muted); cursor: pointer; white-space: nowrap;
  }
  .fbtn:hover { color: var(--text-primary); }
  .fbtn.on {
    color: var(--accent); border-color: color-mix(in srgb, var(--accent) 55%, transparent);
    background: color-mix(in srgb, var(--accent) 10%, transparent);
  }
  .fbtn:disabled { opacity: 0.6; cursor: wait; }

  .empty { padding-top: var(--space-5); color: var(--text-muted); font-size: var(--fs-small); }
  .inline-link { color: var(--accent); font-weight: var(--fw-semibold); margin-left: 6px; }

  @media (max-width: 560px) {
    .meta { display: none; }
  }
</style>
