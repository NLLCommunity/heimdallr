<script lang="ts">
  import { onMount } from "svelte";
  import { querystring, replace } from "svelte-spa-router";
  import { authClient } from "../api/client";
  import { userStore } from "../stores/user.svelte";

  const user = userStore();

  let error = $state<string | null>(null);

  onMount(async () => {
    const params = new URLSearchParams($querystring ?? "");
    const code = params.get("code");

    if (!code) {
      error = "No authorization code received from Discord.";
      return;
    }

    try {
      const res = await authClient.exchangeCode({ code });
      if (res.user) {
        user.user = res.user;
      }
      push("/guilds");
    } catch (e: any) {
      error = e.message;
    }
  });
</script>

<div class="flex min-h-screen items-center justify-center">
  {#if error}
    <div class="text-center">
      <div class="alert alert-error mb-4">
        <span>{error}</span>
      </div>
      <a href="/#/" class="btn btn-ghost">Back to Login</a>
    </div>
  {:else}
    <div class="text-center">
      <span class="loading loading-spinner loading-lg"></span>
      <p class="mt-4">Logging in...</p>
    </div>
  {/if}
</div>
