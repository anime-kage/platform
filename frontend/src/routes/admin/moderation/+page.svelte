<script lang="ts">
  import { onMount } from 'svelte';
  import api from '$lib/api';
  import { authStore as auth } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import type { AdminReportedComment, AdminUser, EpisodeReport } from '$shared/types';

  // Moderation:
  // the report queue + the user manager. Promoting someone into the team
  // happens here too — the Echipă tab only manages existing members.
  const canModerate = $derived(
    $auth.isAuthenticated && ['admin', 'moderator'].includes($auth.user?.role ?? '')
  );
  const isAdmin = $derived($auth.user?.role === 'admin');

  const PAGE = 20;
  let reports = $state<AdminReportedComment[]>([]);
  let total = $state(0);
  let pageIdx = $state(0);
  let loading = $state(true);
  let userQ = $state('');
  let users = $state<AdminUser[]>([]);
  let busy = $state(false);

  // Episode reports: a separate queue from comment reports because they are a
  // different job — these are fixed by editing the episode, not by moderating
  // a person, so the row links straight to it rather than to a profile.
  let epReports = $state<EpisodeReport[]>([]);
  let epLoading = $state(true);

  const pages = $derived(Math.max(1, Math.ceil(total / PAGE)));

  onMount(() => {
    loadReports();
    loadEpisodeReports();
  });

  async function loadEpisodeReports() {
    epLoading = true;
    try {
      epReports = (await api.listEpisodeReports('open')).data;
    } catch {
      toast.error('Rapoartele de episoade nu au putut fi încărcate.');
    } finally {
      epLoading = false;
    }
  }

  /** Link to the episode itself — the point of the queue is to get there in one
   *  click. Slug when we have one, numeric id otherwise (the API 301s it). */
  const epHref = (r: EpisodeReport) =>
    `/anime/${r.animeSlug ?? r.animeId}/episode/${r.episodeNumber}`;

  async function loadReports() {
    loading = true;
    try {
      const res = await api.getAdminReports({ limit: PAGE, offset: pageIdx * PAGE });
      reports = res.data;
      total = res.total;
      // resolving the last report on a page can leave it past the end
      if (pageIdx > 0 && reports.length === 0 && total > 0) {
        pageIdx = Math.min(pageIdx, Math.ceil(total / PAGE) - 1);
        return loadReports();
      }
    } catch {
      toast.error('Rapoartele nu au putut fi încărcate.');
    } finally {
      loading = false;
    }
  }

  function goPage(delta: number) {
    pageIdx = Math.min(Math.max(0, pageIdx + delta), pages - 1);
    loadReports();
  }

  async function act(fn: () => Promise<unknown>, okMsg: string) {
    busy = true;
    try {
      await fn();
      toast.success(okMsg);
      await loadReports();
      if (userQ.trim()) await searchUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Acțiunea a eșuat.');
    } finally {
      busy = false;
    }
  }

  async function searchUsers(e?: SubmitEvent) {
    e?.preventDefault();
    if (!userQ.trim()) return;
    try {
      users = (await api.findUsers(userQ.trim())).data;
    } catch {
      toast.error('Căutarea utilizatorilor a eșuat.');
    }
  }

  async function changeRole(u: AdminUser, e: Event) {
    const role = (e.currentTarget as HTMLSelectElement).value;
    busy = true;
    try {
      await api.setUserRole(u.id, role);
      toast.success(`${u.username} → ${role}`);
      await searchUsers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Schimbarea rolului a eșuat.');
      await searchUsers();
    } finally {
      busy = false;
    }
  }

  const fmtWhen = (iso: string) =>
    new Date(iso).toLocaleDateString('ro-RO', { day: 'numeric', month: 'short' }) +
    ', ' +
    new Date(iso).toLocaleTimeString('ro-RO', { hour: '2-digit', minute: '2-digit' });
</script>

{#if !canModerate}
  <div class="empty">
    <p class="muted">Moderarea e rezervată moderatorilor și administratorilor.</p>
  </div>
{:else}
  <section class="sect">
    <h2 class="s-label">
      Episoade raportate <span class="s-count">· {epReports.length} deschise</span>
    </h2>
    {#if epLoading}
      <p class="muted">Se încarcă…</p>
    {:else if epReports.length === 0}
      <div class="calm">Niciun episod raportat.</div>
    {:else}
      <div class="listcard">
        {#each epReports as r (r.id)}
          <article class="report">
            <div class="r-head">
              <a class="r-user" href={epHref(r)}>{r.animeTitle} · Ep. {r.episodeNumber}</a>
              {#if r.reporter}<span class="r-where">de {r.reporter}</span>{/if}
              <span class="r-when">{fmtWhen(r.createdAt)}</span>
            </div>
            <p class="r-content">{r.body}</p>
            <div class="r-actions">
              <a class="btn ghost sm" href={epHref(r)}>Deschide episodul</a>
              <button class="btn ghost sm" disabled={busy}
                onclick={() => act(async () => {
                  await api.resolveEpisodeReport(r.id);
                  await loadEpisodeReports();
                }, 'Raport marcat ca rezolvat.')}>
                Marchează rezolvat
              </button>
            </div>
          </article>
        {/each}
      </div>
    {/if}
  </section>

  <section class="sect">
    <h2 class="s-label">Comentarii raportate <span class="s-count">· {total} deschise</span></h2>
    {#if loading}
      <p class="muted">Se încarcă…</p>
    {:else if reports.length === 0}
      <div class="calm">Nicio raportare de rezolvat. Liniște.</div>
    {:else}
      <div class="listcard">
        {#each reports as rep (rep.id)}
          <article class="report">
            <div class="r-head">
              <span class="r-user">{rep.username}</span>
              <span class="pill" class:dead={rep.userBanned}>{rep.userRole}{rep.userBanned ? ' · banat' : ''}</span>
              {#if rep.contextTitle}
                <span class="r-where">
                  pe {#if rep.animeId}<a href={`/anime/${rep.animeId}`}>{rep.contextTitle}</a>{:else if rep.mangaId}<a href={`/manga/${rep.mangaId}`}>{rep.contextTitle}</a>{:else}{rep.contextTitle}{/if}
                </span>
              {/if}
              <span class="r-when">{fmtWhen(rep.createdAt)}</span>
            </div>
            <p class="r-content">{rep.content}</p>
            <div class="r-actions">
              <button class="btn ghost sm" disabled={busy}
                onclick={() => act(() => api.dismissReport(rep.id), 'Raport respins. Comentariul rămâne.')}>
                Respinge raportul
              </button>
              <button class="btn ghost sm danger" disabled={busy}
                onclick={() => act(() => api.adminDeleteComment(rep.id), 'Comentariu șters.')}>
                Șterge comentariul
              </button>
              {#if !rep.userBanned && !['admin', 'moderator'].includes(rep.userRole)}
                <button class="btn ghost sm danger" disabled={busy}
                  onclick={() => confirm(`Banezi utilizatorul ${rep.username}?`) &&
                    act(() => api.setUserBan(rep.userId, true), `${rep.username} a fost banat.`)}>
                  Banează autorul
                </button>
              {/if}
            </div>
          </article>
        {/each}
      </div>
      {#if pages > 1}
        <div class="pager">
          <button class="btn ghost sm" disabled={busy || pageIdx === 0} onclick={() => goPage(-1)}>← Anterior</button>
          <span class="pager-at">pagina {pageIdx + 1} din {pages}</span>
          <button class="btn ghost sm" disabled={busy || pageIdx >= pages - 1} onclick={() => goPage(1)}>Următor →</button>
        </div>
      {/if}
    {/if}
  </section>

  <section class="sect">
    <h2 class="s-label">Utilizatori</h2>
    <form class="usearch" onsubmit={searchUsers}>
      <input type="search" bind:value={userQ} placeholder="Caută după nume de utilizator…" />
      <button class="btn fill" type="submit">Caută</button>
    </form>
    {#if users.length > 0}
      <div class="listcard">
        {#each users as u (u.id)}
          <div class="urow">
            <span class="r-user">{u.username}{u.id === $auth.user?.id ? ' (tu)' : ''}</span>
            {#if isAdmin && u.id !== $auth.user?.id}
              <select class="roleselect" value={u.role} disabled={busy} onchange={(e) => changeRole(u, e)}>
                <option value="user">user</option>
                <option value="translator">traducător</option>
                <option value="verifier">verificator</option>
                <option value="coordinator">coordonator</option>
                <option value="moderator">moderator</option>
                <option value="admin">admin</option>
              </select>
            {:else}
              <span class="rolefixed" class:accent={u.role === 'admin'}>{u.role}</span>
            {/if}
            {#if u.isBanned}<span class="pill dead">banat</span>{/if}
            <span class="spacer"></span>
            {#if !['admin', 'moderator'].includes(u.role) && u.id !== $auth.user?.id}
              <button class="btn ghost sm" class:danger={!u.isBanned} disabled={busy}
                onclick={() => (u.isBanned || confirm(`Banezi utilizatorul ${u.username}?`)) &&
                  act(() => api.setUserBan(u.id, !u.isBanned), u.isBanned ? 'Ban ridicat.' : 'Utilizator banat.')}>
                {u.isBanned ? 'Ridică banul' : 'Banează'}
              </button>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
    <p class="foot-note">
      De aici promovezi un utilizator în echipă (caută-l, schimbă-i rolul) — membrii existenți
      se administrează din tabul Echipă.
    </p>
  </section>
{/if}

<style>
  .sect { margin-bottom: var(--space-6); }
  .s-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted);
    padding-bottom: 12px; border-bottom: 1px solid var(--border-default);
    margin-bottom: var(--space-4);
  }
  .s-count { color: var(--text-muted); font-weight: var(--fw-regular); letter-spacing: 0.06em; text-transform: none; }

  .calm {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-5); text-align: center; color: var(--text-muted); font-size: var(--fs-small);
  }

  .listcard {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }

  /* report rows */
  .report { padding: var(--space-4) var(--space-5); }
  .report + .report { border-top: 1px solid var(--border-subtle); }
  .r-head { display: flex; align-items: baseline; gap: 11px; flex-wrap: wrap; }
  .r-user { font-weight: var(--fw-semibold); color: var(--text-primary); }
  .r-where { font-size: var(--fs-caption); color: var(--text-muted); }
  .r-where a { color: var(--text-muted); text-decoration: underline; }
  .r-where a:hover { color: var(--text-primary); }
  .r-when { font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted); margin-left: auto; white-space: nowrap; }
  /* the reported comment, shown as it appears on the site — no quote dressing */
  .r-content {
    font-size: var(--fs-small); color: var(--text-primary);
    line-height: 1.55; margin: 10px 0 0; max-width: 64ch; white-space: pre-wrap;
  }
  .r-actions { display: flex; gap: 9px; flex-wrap: wrap; margin-top: 13px; }

  .pager { display: flex; align-items: center; justify-content: center; gap: 14px; margin-top: var(--space-3); }
  .pager-at { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  /* user rows */
  .usearch { display: flex; gap: 8px; margin-bottom: var(--space-3); }
  .usearch input {
    flex: 1; min-height: 44px; padding: 0 14px;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-md); color: var(--text-primary); outline: none;
  }
  .usearch input:focus { border-color: var(--accent); }
  .urow { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; padding: 11px 18px; }
  .urow + .urow { border-top: 1px solid var(--border-subtle); }
  .urow:hover { background: var(--surface-overlay); }
  .spacer { flex: 1; }
  .roleselect {
    min-height: 34px; padding: 0 8px;
    font-family: var(--font-mono); font-size: var(--fs-caption);
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); cursor: pointer;
  }
  .rolefixed { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .rolefixed.accent { color: var(--accent); }

  .pill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.08em; text-transform: uppercase; white-space: nowrap;
    color: var(--text-muted);
  }
  .pill.dead { color: var(--danger); }

  .foot-note {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    margin-top: 12px; line-height: 1.6;
  }

  .empty {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-5); text-align: center;
  }
  .muted { color: var(--text-muted); }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 10px 18px; border-radius: var(--radius-md); white-space: nowrap; cursor: pointer;
    transition: background var(--motion-fast) var(--ease), border-color var(--motion-fast) var(--ease);
  }
  .btn.sm { padding: 7px 13px; font-size: var(--fs-caption); }
  .btn.fill { background: var(--accent); color: var(--on-accent); border: none; }
  .btn.fill:hover { background: var(--accent-hover); }
  .btn.ghost { border: 1px solid var(--border-default); background: transparent; color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-overlay); border-color: var(--border-strong); }
  .btn.ghost.danger { color: var(--danger); border-color: color-mix(in srgb, var(--danger) 40%, transparent); }
  .btn.ghost.danger:hover { background: color-mix(in srgb, var(--danger) 12%, transparent); }
  .btn:disabled { opacity: 0.6; cursor: wait; }
</style>
