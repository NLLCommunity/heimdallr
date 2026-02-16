<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import { authClient } from "../api/client";
  import { isLoggedIn } from "../auth/auth";
  import { guildsStore } from "../stores/guilds.svelte";
  import { userStore } from "../stores/user.svelte";
  import Layout from "../components/layout/Layout.svelte";

  const guilds = guildsStore();
  const user = userStore();

  let error = $state<string | null>(null);

  onMount(async () => {
    if (!isLoggedIn()) {
      push("/");
      return;
    }

    if (!user.user) {
      try {
        const res = await authClient.getCurrentUser({});
        if (res.user) {
          user.user = res.user;
        }
      } catch {
        push("/");
        return;
      }
    }

    guilds.loading = true;
    try {
      const res = await authClient.listGuilds({});
      guilds.guilds = res.guilds;
    } catch (e: any) {
      error = e.message;
    } finally {
      guilds.loading = false;
    }
  });
</script>

<Layout>
  <div class="mx-auto max-w-4xl">
    <h1 class="mb-6 text-3xl font-bold">Select a Server</h1>

    {#if error}
      <div class="alert alert-error mb-4">
        <span>{error}</span>
      </div>
    {/if}

    {#if guilds.loading}
      <div class="flex justify-center py-12">
        <span class="loading loading-spinner loading-lg"></span>
      </div>
    {:else if guilds.guilds.length === 0}
      <div class="text-center py-12">
        <p class="text-base-content/60">
          No servers found. Make sure you have admin permissions in a server where Heimdallr is installed.
        </p>
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
        {#each guilds.guilds as guild}
          <a
            href="/#/guild/{guild.id}"
            class="card bg-base-100 shadow-md transition-shadow hover:shadow-lg"
          >
            <div class="card-body items-center text-center">
              {#if guild.icon}
                <div class="avatar">
                  <div class="w-16 rounded-full">
                    <img
                      src="https://cdn.discordapp.com/icons/{guild.id}/{guild.icon}.png?size=128"
                      alt={guild.name}
                    />
                  </div>
                </div>
              {:else}
                <div class="avatar placeholder">
                  <div class="bg-neutral text-neutral-content w-16 rounded-full">
                    <span class="text-xl">{guild.name.charAt(0).toUpperCase()}</span>
                  </div>
                </div>
              {/if}
              <h2 class="card-title text-base">{guild.name}</h2>
            </div>
          </a>
        {/each}
      </div>
    {/if}
  </div>
</Layout>
