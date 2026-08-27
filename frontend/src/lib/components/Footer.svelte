<script lang="ts">
  const year = new Date().getFullYear();
</script>

<!--
  One band, on purpose. This used to be two: a grid of link columns
  (Explorează / Comunitate / Cont) above a legal strip, which read as two
  stacked footers. The columns are gone rather than restyled — every link in
  them pointed at a members-only route, so on the landing page, the one page a
  guest ever sees, all of them bounced straight back to it.

  What is left is the brand and the legal line, merged into a single row.
-->
<footer class="footer">
  <div class="container inner">
    <div class="brand-col">
      <a class="brand" href="/">
        <img src="/logo.png" alt="" width="30" height="30" />
        <span class="wordmark">Anime<span class="dot">·</span>Kage</span>
      </a>
      <p class="blurb">Anime și manga subtitrate în română.</p>
    </div>

    <div class="legal">
      <span>© {year} Anime·Kage</span>
      <span>Proiect din pasiune · non-profit</span>
    </div>
  </div>
</footer>

<style>
  .footer { border-top: 1px solid var(--border-subtle); margin-top: var(--space-6); }

  .inner {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: var(--space-5);
    padding-block: var(--space-6);
  }

  .brand { display: flex; align-items: center; gap: 10px; }
  .brand img { width: 30px; height: 30px; object-fit: contain; }
  .wordmark {
    font-family: var(--font-display); font-weight: var(--fw-semibold);
    font-size: 1.125rem; color: var(--text-primary);
  }
  .dot { color: var(--accent); }
  .blurb {
    max-width: 280px; margin-top: var(--space-4);
    font-size: var(--fs-small); line-height: 1.6; color: var(--text-muted);
  }

  /* Stacked on the right at wide widths, so the two lines read as one block
     rather than drifting apart across the full container. */
  .legal {
    display: flex; flex-direction: column; align-items: flex-end; gap: 6px;
    font-family: var(--font-mono); font-size: var(--fs-caption); color: var(--text-muted);
    text-align: right;
  }

  @media (max-width: 760px) {
    .inner { flex-direction: column; align-items: flex-start; }
    .legal { align-items: flex-start; text-align: left; }
  }
</style>
