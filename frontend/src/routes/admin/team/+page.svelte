<script lang="ts">
  import { api } from '$lib/api';
  import { nameHue } from '$lib/avatar';
  import { authStore as auth } from '$lib/stores/auth';
  import { toast } from '$lib/stores/toast';
  import type { TeamMember } from '$shared/types';

  // The team roster:
  // everyone with a team role, role changes inline. Bans and plain-user
  // promotions live in Moderare — this page is about the working team.
  const canModerate = $derived(
    $auth.isAuthenticated && ['admin', 'moderator'].includes($auth.user?.role ?? '')
  );
  const isAdmin = $derived($auth.user?.role === 'admin');

  let members = $state<TeamMember[]>([]);
  let loading = $state(true);
  let busy = $state(false);

  let loaded = false;
  $effect(() => {
    if (!canModerate || loaded) return;
    loaded = true;
    load();
  });

  async function load() {
    loading = true;
    api
      .getReleaseQuota()
      .then((q) => (defaultCap = q.data.limit))
      .catch(() => {});
    try {
      members = (await api.getAdminTeam()).data;
    } catch {
      toast.error('Echipa nu a putut fi încărcată.');
    } finally {
      loading = false;
    }
  }

  async function changeRole(m: TeamMember, e: Event) {
    const role = (e.currentTarget as HTMLSelectElement).value;
    busy = true;
    try {
      await api.setUserRole(m.id, role);
      toast.success(`${m.username} → ${role}`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Schimbarea rolului a eșuat.');
    } finally {
      busy = false;
      await load();
    }
  }

  // Coordinators and admins drain the queue — capping them is the wrong
  // failure mode, so the server exempts them and the column says so.
  const capExempt = (role: string) => role === 'admin' || role === 'coordinator';
  // the server-wide default, read from the server rather than hardcoded here:
  // for an exempt caller the quota endpoint reports the configured limit, so
  // changing TRANSLATOR_RELEASE_CAP does not silently make this label lie
  let defaultCap = $state(10);
  const effectiveCap = (m: TeamMember) => m.releaseCap ?? defaultCap;

  async function changeCap(m: TeamMember, e: Event) {
    const raw = (e.currentTarget as HTMLInputElement).value.trim();
    const cap = raw === '' ? null : Number(raw);
    if (cap !== null && (!Number.isInteger(cap) || cap < 0)) {
      toast.error('Limita trebuie să fie un număr întreg pozitiv (sau gol).');
      return;
    }
    busy = true;
    try {
      await api.setUserReleaseCap(m.id, cap);
      toast.success(
        cap === null
          ? `${m.username} → limita implicită (${defaultCap})`
          : cap === 0
            ? `${m.username} → fără limită`
            : `${m.username} → ${cap} episoade`
      );
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Schimbarea limitei a eșuat.');
    } finally {
      busy = false;
      await load();
    }
  }

  const fmtSince = (iso: string) =>
    new Date(iso).toLocaleDateString('ro-RO', { month: 'short', year: 'numeric' });

  const roleLabels: Record<string, string> = {
    translator: 'traducător',
    verifier: 'verificator',
    coordinator: 'coordonator',
    moderator: 'moderator',
    admin: 'admin'
  };

  const roleCards = [
    { name: 'user', desc: 'Vede tot conținutul, scrie recenzii și comentează. Rolul implicit.' },
    { name: 'traducător', desc: 'Creează release-uri și traduce în editor. Nu poate publica direct.' },
    { name: 'verificator', desc: 'Verifică release-urile, corectează, aprobă sau returnează.' },
    { name: 'coordonator', desc: 'Publică release-urile aprobate: confirmă seria, importă titluri din MAL, atașează sursele.' },
    { name: 'moderator', desc: 'Moderare comentarii și utilizatori, plus tot pipeline-ul.' },
    { name: 'admin', desc: 'Catalog, roluri și moderare. Vede și poate face tot.' }
  ];
</script>

{#if !canModerate}
  <div class="empty">
    <p class="muted">Pagina echipei e rezervată moderatorilor și administratorilor.</p>
  </div>
{:else}
  <div class="cols">
    <div class="main">
      <h2 class="s-label">Membri <span class="s-count">· {members.length}</span></h2>
      {#if loading}
        <p class="muted">Se încarcă…</p>
      {:else}
        <div class="listcard">
          <div class="mrow head">
            <span class="h">membru</span>
            <span class="h">rol</span>
            <span class="h" title="Episoade neterminate / limită">în lucru</span>
            <span class="h">în echipă din</span>
            <span class="h"></span>
          </div>
          {#each members as m (m.id)}
            <div class="mrow">
              <div class="who">
                <span class="avatar monogram" style="--mg-hue: {nameHue(m.username)}">
                  {m.username[0]?.toUpperCase()}
                </span>
                <div class="who-main">
                  <span class="name">{m.username}{m.id === $auth.user?.id ? ' (tu)' : ''}</span>
                </div>
              </div>
              <div>
                {#if isAdmin && m.id !== $auth.user?.id}
                  <select class="roleselect" value={m.role} disabled={busy} onchange={(e) => changeRole(m, e)}>
                    <option value="user">user</option>
                    <option value="translator">traducător</option>
                    <option value="verifier">verificator</option>
                    <option value="coordinator">coordonator</option>
                    <option value="moderator">moderator</option>
                    <option value="admin">admin</option>
                  </select>
                {:else}
                  <span class="rolefixed" class:accent={m.role === 'admin'}>{roleLabels[m.role] ?? m.role}</span>
                {/if}
              </div>
              <!-- the cap bounds staging: translators × cap × file size. It is
                   meaningless without the in-flight count next to it. -->
              <div class="cap">
                {#if capExempt(m.role)}
                  <span class="cap-none" title="Coordonatorii și adminii nu au limită">—</span>
                {:else}
                  <span class="cap-used" class:full={m.inFlight >= effectiveCap(m)}>{m.inFlight}</span>
                  <span class="cap-sep">/</span>
                  {#if isAdmin}
                    <input
                      class="cap-in"
                      type="number"
                      min="0"
                      max="1000"
                      disabled={busy}
                      value={m.releaseCap ?? ''}
                      placeholder={String(defaultCap)}
                      title="Gol = limita implicită ({defaultCap}). 0 = fără limită."
                      onchange={(e) => changeCap(m, e)}
                    />
                  {:else}
                    <span class="cap-used">{effectiveCap(m) || '∞'}</span>
                  {/if}
                {/if}
              </div>
              <span class="since">{fmtSince(m.createdAt)}</span>
              <span class="flags">
                {#if m.isBanned}<span class="pill dead">banat</span>{/if}
              </span>
            </div>
          {/each}
          {#if members.length === 0}
            <div class="mrow"><span class="muted">Nimeni în echipă încă — dă roluri din Moderare.</span></div>
          {/if}
        </div>
        <p class="foot-note">
          Rolul se schimbă direct din listă și se aplică imediat. Promovarea unui utilizator
          obișnuit se face din Moderare — caută-l după nume.
        </p>
        <p class="foot-note">
          <strong>În lucru</strong> arată episoadele neterminate și limita fiecăruia. Spațiul de
          pe server e traducători × limită × mărimea fișierului, așa că limita e singurul lucru
          care ține discul previzibil. Gol = limita implicită ({defaultCap}); 0 = fără limită.
          Capitolele de manga nu intră la socoteală.
        </p>
      {/if}
    </div>

    <aside class="rail">
      <section class="r-sect">
        <h2 class="r-label">Ce poate fiecare rol</h2>
        {#each roleCards as rc (rc.name)}
          <div class="rolecard">
            <span class="rc-name">{rc.name}</span>
            <p class="rc-desc">{rc.desc}</p>
          </div>
        {/each}
      </section>
    </aside>
  </div>
{/if}

<style>
  .cols {
    display: grid; grid-template-columns: minmax(0, 1fr) 280px;
    gap: var(--space-7); align-items: start;
  }
  @media (max-width: 900px) { .cols { grid-template-columns: minmax(0, 1fr); } }

  .s-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--text-muted);
    padding-bottom: 12px; border-bottom: 1px solid var(--border-default);
    margin-bottom: var(--space-4);
  }
  .s-count { color: var(--text-muted); font-weight: var(--fw-regular); }

  .listcard {
    border: 1px solid var(--border-subtle); border-radius: var(--radius-lg);
    background: var(--surface-raised); overflow: hidden;
  }
  /* five columns inside the narrow main column of a two-column page: keep the
     fixed tracks tight or the name column collapses under the avatar */
  .mrow {
    display: grid; grid-template-columns: minmax(120px, 1.4fr) 128px 88px 92px 34px;
    gap: var(--space-3); align-items: center; padding: 12px 14px;
  }
  .cap { display: flex; align-items: center; gap: 5px; font-family: var(--font-mono); }
  .cap-used { font-size: var(--fs-caption); font-weight: var(--fw-semibold); }
  .cap-used.full { color: var(--danger); }
  .cap-sep, .cap-none { color: var(--text-muted); font-size: var(--fs-caption); }
  .cap-in {
    width: 52px; min-height: 30px; padding: 0 6px;
    font-family: var(--font-mono); font-size: var(--fs-caption); text-align: center;
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); outline: none;
  }
  .cap-in:focus { border-color: var(--accent); }
  .cap-in:disabled { opacity: 0.6; }
  .mrow + .mrow { border-top: 1px solid var(--border-subtle); }
  .mrow:not(.head):hover { background: var(--surface-overlay); }
  .mrow.head { padding-block: 10px; border-bottom: 1px solid var(--border-default); }
  .h {
    font-family: var(--font-mono); font-size: var(--fs-micro);
    letter-spacing: 0.1em; text-transform: uppercase; color: var(--text-muted);
  }

  .who { display: flex; align-items: center; gap: 11px; min-width: 0; }
  .avatar { width: 32px; height: 32px; flex: 0 0 auto; font-size: var(--fs-caption); }
  .who-main { min-width: 0; }
  .name {
    font-size: var(--fs-small); font-weight: var(--fw-semibold);
    white-space: nowrap; overflow: hidden; text-overflow: ellipsis; display: block;
  }

  .roleselect {
    width: 100%; max-width: 128px; min-height: 34px; padding: 0 6px;
    font-family: var(--font-mono); font-size: var(--fs-caption);
    background: var(--surface-inset); border: 1px solid var(--border-default);
    border-radius: var(--radius-sm); color: var(--text-primary); cursor: pointer;
  }
  .rolefixed { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }
  .rolefixed.accent { color: var(--accent); }
  .since { font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted); }

  .pill {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.06em; text-transform: uppercase;
    padding: 3px 10px; border-radius: var(--radius-pill); white-space: nowrap;
  }
  .pill.dead { background: color-mix(in srgb, var(--danger) 14%, transparent); color: var(--danger); }

  .foot-note {
    font-family: var(--font-mono); font-size: var(--fs-micro); color: var(--text-muted);
    margin-top: 12px; line-height: 1.6;
  }

  .rail { position: sticky; top: calc(var(--header-h) + var(--space-4)); }
  @media (max-width: 900px) { .rail { position: static; } }
  .r-sect { margin-bottom: var(--space-6); }
  .r-label {
    font-family: var(--font-mono); font-size: var(--fs-micro); font-weight: var(--fw-semibold);
    letter-spacing: 0.14em; text-transform: uppercase; color: var(--accent);
    padding-bottom: 12px;
  }
  .rolecard { padding: 11px 0; border-top: 1px solid var(--border-subtle); }
  .rc-name { font-size: var(--fs-small); font-weight: var(--fw-bold); }
  .rc-desc { font-size: var(--fs-caption); color: var(--text-muted); line-height: 1.55; margin-top: 4px; }

  .empty {
    border: 1px dashed var(--border-default); border-radius: var(--radius-md);
    padding: var(--space-5); text-align: center;
  }
  .muted { color: var(--text-muted); }

  @media (max-width: 640px) {
    .mrow { grid-template-columns: minmax(0, 1fr) 120px 88px; }
    .since, .flags, .h:nth-child(4), .h:nth-child(5) { display: none; }
  }
</style>
