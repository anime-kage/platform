<script lang="ts">
  import { goto } from '$app/navigation';
  import ReleaseEditor from '$lib/components/ReleaseEditor.svelte';
  import { authStore as auth } from '$lib/stores/auth';

  let { data } = $props();

  // translators (and admins) work here; verifiers/moderators get the same
  // release in review mode at /verify/[id] — old links redirect over
  $effect(() => {
    if ($auth.isLoading) return;
    const role = $auth.user?.role ?? '';
    if (!$auth.isAuthenticated) goto(`/login?redirect=/translate/${data.releaseId}`);
    else if (['verifier', 'moderator'].includes(role)) goto(`/verify/${data.releaseId}`);
    else if (!['translator', 'admin'].includes(role)) goto('/translate');
  });
</script>

<svelte:head>
  <title>Editor · Anime-Kage</title>
</svelte:head>

<ReleaseEditor releaseId={data.releaseId} mode="translate" />
