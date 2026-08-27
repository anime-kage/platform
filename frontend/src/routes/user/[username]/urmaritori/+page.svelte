<script lang="ts">
  import NetworkList from '$lib/components/NetworkList.svelte';

  let { data } = $props();
</script>

<NetworkList kind="followers" handle={data.handle} name={data.name} isReal={data.kind === 'real'} rows={data.rows} />
