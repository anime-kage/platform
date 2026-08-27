<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import api from '$lib/api';

  onMount(async () => {
    try {
      const response = await api.getRandomAnime();
      if (response.data?.id) {
        goto(`/anime/${response.data.id}`, { replaceState: true });
        return;
      }
    } catch (err) {
      console.error('Failed to get random anime:', err);
    }
    goto('/anime', { replaceState: true });
  });
</script>

<div class="loading-container">
  <div class="spinner"></div>
  <p class="loading-text">Se alege un anime aleatoriu...</p>
</div>

<style>
  .loading-container {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    min-height: 60vh;
    gap: 1rem;
  }

  .spinner {
    width: 3rem;
    height: 3rem;
    border: 3px solid rgba(54, 171, 179, 0.2);
    border-top-color: #36abb3;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .loading-text {
    color: rgba(255, 255, 255, 0.7);
    font-size: 1rem;
  }
</style>
