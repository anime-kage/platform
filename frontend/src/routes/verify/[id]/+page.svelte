<script lang="ts">
  import { goto } from '$app/navigation';
  import ReleaseEditor from '$lib/components/ReleaseEditor.svelte';
  import { authStore as auth } from '$lib/stores/auth';

  let { data } = $props();

  // the review gate is verifier+ (coordinators preview here before publishing,
  // admins included); translators belong in their own editor
  $effect(() => {
    if ($auth.isLoading) return;
    const role = $auth.user?.role ?? '';
    if (!$auth.isAuthenticated) goto(`/login?redirect=/verify/${data.releaseId}`);
    else if (role === 'translator') goto(`/translate/${data.releaseId}`);
    else if (!['verifier', 'coordinator', 'moderator', 'admin'].includes(role)) goto('/verify');
  });
</script>

<svelte:head>
  <title>Verificare · Anime-Kage</title>
</svelte:head>

<ReleaseEditor releaseId={data.releaseId} mode="verify" />
