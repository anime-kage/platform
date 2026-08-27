<script lang="ts">
  import { mediaUrl } from '$lib/media';
  import { goto } from '$app/navigation';
  import { authStore } from '$lib/stores/auth';
  import { displayName } from '$lib/types';
  // import TestNotice from '$lib/components/TestNotice.svelte';

  let { data } = $props();
  const auth = $derived($authStore);

  // Members land on the dashboard; the landing is the public front door.
  $effect(() => {
    if (!auth.isLoading && auth.isAuthenticated) goto('/home', { replaceState: true });
  });

  // The Discord server is the only entrance: with INVITE_ONLY on, registering
  // needs a single-use code that only the bot's /invitatie mints. Every "ask for
  // access" route on this page therefore leads to Discord, not inward — the
  // register page is for people who already hold a code.
  const DISCORD_INVITE = 'https://discord.com/invite/dWWWdcrrQk';

  const LAYOUTS = [
    { left: '8%', top: '2%', width: '200px', rot: '-5deg', z: 2 },
    { left: '40%', top: '18%', width: '210px', rot: '3deg', z: 3 },
    { left: '18%', top: '46%', width: '190px', rot: '-2deg', z: 1 }
  ];
</script>

<svelte:head><title>Anime-Kage</title></svelte:head>

<!-- The same popup the home page shows, on its own "seen" flag. The permanent
     notice in the hero stays: the popup catches people who skim past copy, the
     copy catches people who dismiss modals on reflex. -->
<!-- Commented out for the 1.0 launch: the catalogue has published content now, so the warning is no longer true. Restore this (and the import above) if the site goes back into a testing phase.
<TestNotice scope="landing" /> -->

<!-- HERO -->
<section class="hero">
  <div class="hero-inner">
    <div class="hero-copy anim-up">
      <p class="hero-kicker">Comunitate pe bază de invitație</p>
      <h1 class="hero-title">
        Anime și manga,<br />traduse cu grijă<br />în <em>română</em>.
      </h1>
      <p class="hero-sub">
        Un loc liniștit pentru cei care iubesc povestea. Urmărește, notează,
        construiește liste și discută cu o comunitate restrânsă de fani.
      </p>
      <!-- Commented out for the 1.0 launch. It was permanent, unlike the home
           page popup, so that a visitor who had never signed in learned there
           was no content yet *before* asking for an invitation. That is no
           longer true. Restore it verbatim if the site goes back into testing.
      <p class="testing">
        <strong>Site-ul este în test.</strong> Încă nu există anime-uri sau manga publicate.
        Momentan îți poți face cont și explora; conținutul apare pe măsură ce echipa îl
        termină.
      </p>
      -->

      <div class="hero-actions">
        <a class="btn fill lg" href="/register">Am o invitație</a>
        <a class="btn ghost lg" href={DISCORD_INVITE} target="_blank" rel="noopener noreferrer">
          Cere acces pe Discord
        </a>
      </div>
    </div>

    <div class="collage" aria-hidden="true">
      {#each data.collage as a, i (a.id)}
        {@const L = LAYOUTS[i]}
        <div
          class="collage-card"
          style={`left:${L.left};top:${L.top};width:${L.width};transform:rotate(${L.rot});z-index:${L.z}`}
        >
          {#if a.imageUrl}
            <img class="collage-art media-tone" src={mediaUrl(a.imageUrl)} alt="" loading="lazy" />
          {/if}
          <span class="collage-veil">
            <span class="collage-t">{displayName(a)}</span>
          </span>
        </div>
      {/each}
    </div>
  </div>
</section>

<!-- ACCES PE BAZĂ DE INVITAȚIE -->
<section class="invite">
  <div class="invite-head">
    <h2>Acces pe bază de invitație</h2>
    <span class="spacer"></span>
    <span class="kicker">fără liste de așteptare</span>
  </div>

  <div class="steps">
    <div class="step">
      <p class="step-n">01</p>
      <h3>Intră pe Discord</h3>
      <p class="step-p">
        Alătură-te serverului comunității. Aici stă toată discuția și tot de
        aici pornesc invitațiile.
      </p>
    </div>
    <div class="step bordered">
      <p class="step-n">02</p>
      <h3>Generează un cod cu botul</h3>
      <p class="step-p">
        Scrie comanda botului și primești pe loc un cod unic, valabil o
        singură dată.
      </p>
      <p class="code-demo">
        <span class="code-cmd">/invitatie</span>
        <span class="code-arrow">→</span>
        <span class="code-out">KAGE‑7F2A‑9X</span>
      </p>
    </div>
    <div class="step bordered">
      <p class="step-n">03</p>
      <h3>Creează-ți contul aici</h3>
      <p class="step-p">
        Introdu codul la înregistrare pe Anime·Kage și contul tău e activat
        pe loc.
      </p>
    </div>
  </div>

  <div class="invite-cta">
    <a class="btn fill" href={DISCORD_INVITE} target="_blank" rel="noopener noreferrer">
      Intră pe server
    </a>
    <span class="cta-note">Primești codul în câteva secunde, direct de la bot.</span>
  </div>
</section>

<style>
  /* ---- hero ---- */
  .hero { position: relative; overflow: hidden; }
  .hero-inner {
    position: relative; max-width: var(--container); margin: 0 auto;
    padding: 76px clamp(1rem, 4vw, 2.5rem) 92px;
    display: grid; grid-template-columns: 1.05fr 0.95fr;
    gap: 50px; align-items: center;
  }
  .hero-kicker { font-size: 0.8125rem; font-weight: var(--fw-semibold); color: var(--accent); letter-spacing: 0.01em; }
  .hero-title {
    font-size: var(--fs-display); line-height: 1.04;
    letter-spacing: -0.015em; margin-top: 24px;
  }
  .hero-title em { font-style: italic; color: var(--accent); }
  .hero-sub {
    max-width: 430px; margin-top: 22px;
    font-size: 1.0625rem; line-height: 1.62; color: var(--text-muted);
  }
  /* Styles for the "site is in test" notice above, commented out with it for
     the 1.0 launch. Kept so restoring the notice is one uncomment, not a
     rewrite. Amber rather than red on purpose: it describes a state of the
     project, not a fault — a danger colour reads as "something is broken".
  .testing {
    max-width: 430px; margin-top: 20px;
    padding: 12px 14px;
    border: 1px solid color-mix(in srgb, var(--warning) 38%, transparent);
    border-left-width: 3px;
    border-radius: var(--radius-sm);
    background: color-mix(in srgb, var(--warning) 8%, transparent);
    font-size: var(--fs-caption); line-height: 1.55; color: var(--text-muted);
  }
  .testing strong { color: var(--text-primary); font-weight: var(--fw-semibold); }
  */

  .hero-actions { display: flex; gap: 12px; margin-top: 30px; flex-wrap: wrap; }

  .btn {
    font-weight: var(--fw-semibold); font-size: var(--fs-small);
    padding: 13px 24px; border-radius: var(--radius-md); white-space: nowrap;
  }
  .btn.lg { font-size: var(--fs-body); padding: 14px 24px; border-radius: 11px; }
  .btn.fill { background: var(--accent); color: var(--on-accent); }
  .btn.fill:hover { background: var(--accent-hover); color: var(--on-accent); }
  .btn.ghost { border: 1px solid var(--border-default); color: var(--text-primary); }
  .btn.ghost:hover { background: var(--surface-raised); color: var(--text-primary); }

  /* ---- collage ---- */
  .collage { position: relative; height: 470px; }
  .collage-card {
    position: absolute; aspect-ratio: 2 / 3;
    border-radius: 12px; overflow: hidden;
    border: 1px solid var(--border-default);
    box-shadow: 0 22px 50px rgba(0, 0, 0, 0.45);
    background: var(--surface-overlay);
  }
  .collage-art { width: 100%; height: 100%; object-fit: cover; }
  .collage-veil {
    position: absolute; left: 0; right: 0; bottom: 0;
    padding: 26px 12px 12px;
    background: linear-gradient(to top, rgba(8, 8, 10, 0.92), transparent);
  }
  .collage-t {
    font-family: var(--font-display); font-size: 0.875rem;
    font-weight: var(--fw-semibold); color: #fff; line-height: 1.15;
  }

  /* ---- invite steps ---- */
  .invite {
    max-width: 1040px; margin: 0 auto;
    padding: 8px clamp(1rem, 4vw, 2.5rem) 84px;
  }
  .invite-head {
    display: flex; align-items: baseline; gap: 16px; flex-wrap: wrap;
    padding-bottom: 16px; border-bottom: 2px solid var(--text-primary);
  }
  .invite-head h2 { font-size: 1.625rem; letter-spacing: -0.012em; }
  .spacer { flex: 1; }

  .steps { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .step { padding: 26px 30px 8px 0; }
  .step.bordered { padding-left: 30px; border-left: 1px solid var(--border-subtle); }
  .step:last-child { padding-right: 0; }
  .step-n { font-family: var(--font-mono); font-size: 0.75rem; color: var(--accent); margin-bottom: 12px; }
  .step h3 { font-size: 1.125rem; margin-bottom: 8px; }
  .step-p { font-size: 0.84375rem; line-height: 1.6; color: var(--text-muted); }

  .code-demo {
    display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
    margin-top: 13px; font-family: var(--font-mono); font-size: 0.71875rem;
  }
  .code-cmd {
    padding: 5px 9px; border-radius: 7px;
    background: var(--surface-overlay); border: 1px solid var(--border-default);
    color: var(--text-primary);
  }
  .code-arrow { color: var(--text-muted); }
  .code-out {
    padding: 5px 9px; border-radius: 7px; letter-spacing: 0.04em;
    background: color-mix(in srgb, var(--accent) 12%, transparent);
    color: var(--accent);
  }

  .invite-cta {
    display: flex; align-items: center; gap: 16px; flex-wrap: wrap;
    margin-top: 28px; padding-top: 24px;
    border-top: 1px solid var(--border-subtle);
  }
  .cta-note { font-size: 0.84375rem; color: var(--text-muted); }

  /* ---- responsive ---- */
  @media (max-width: 900px) {
    .hero-inner { grid-template-columns: minmax(0, 1fr); padding-top: 48px; padding-bottom: 56px; }
    .collage { height: 420px; max-width: 440px; }
    .steps { grid-template-columns: minmax(0, 1fr); }
    .step { padding: 26px 0 8px; }
    .step.bordered { padding-left: 0; border-left: none; border-top: 1px solid var(--border-subtle); }
  }
</style>
