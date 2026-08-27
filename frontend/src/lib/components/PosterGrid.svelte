<script lang="ts">
  import PosterCard from './PosterCard.svelte';
  import type { Anime, Manga } from '$lib/types';

  let { items, kind = 'anime' }: { items: (Anime | Manga)[]; kind?: 'anime' | 'manga' } = $props();
</script>

<div class="grid">
  {#each items as a (a.id)}
    <PosterCard {a} href={`/${kind}/${a.id}`} />
  {/each}
</div>

<style>
  .grid {
    display: grid;
    gap: var(--space-5);
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  }
  @media (max-width: 520px) {
    .grid { grid-template-columns: repeat(auto-fill, minmax(120px, 1fr)); gap: var(--space-4); }
  }
</style>
