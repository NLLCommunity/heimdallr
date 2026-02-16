<script lang="ts">
  import { userStore } from "../../stores/user.svelte";
  import { guildsStore } from "../../stores/guilds.svelte";
  import { push } from "svelte-spa-router";
  import { authClient } from "../../api/client";
  import { clearToken } from "../../auth/auth";

  interface Props {
    currentGuildId?: string;
  }

  let { currentGuildId }: Props = $props();

  const user = userStore();
  const guilds = guildsStore();

  const currentGuild = $derived(
    guilds.guilds.find((g) => g.id === currentGuildId)
  );

  async function logout() {
    try {
      await authClient.logout({});
    } catch {
      // Ignore errors, clear token regardless
    }
    clearToken();
    user.user = null;
    push("/");
  }
</script>

<div class="navbar bg-base-200">
  <div class="flex-1">
    <a class="btn btn-ghost text-xl" href="/#/guilds">Heimdallr</a>
    {#if currentGuild}
      <span class="text-base-content/60 ml-2">/ {currentGuild.name}</span>
    {/if}
  </div>
  <div class="flex-none gap-2">
    {#if currentGuildId && guilds.guilds.length > 1}
      <div class="dropdown dropdown-end">
        <div tabindex="0" role="button" class="btn btn-ghost btn-sm">
          Switch Server
        </div>
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <ul tabindex="0" class="dropdown-content menu bg-base-300 rounded-box z-10 w-52 p-2 shadow">
          {#each guilds.guilds.filter((g) => g.id !== currentGuildId) as guild}
            <li>
              <a href="/#/guild/{guild.id}">{guild.name}</a>
            </li>
          {/each}
        </ul>
      </div>
    {/if}
    {#if user.user}
      <div class="dropdown dropdown-end">
        <div tabindex="0" role="button" class="btn btn-ghost btn-circle avatar placeholder">
          {#if user.user.avatar}
            <div class="w-10 rounded-full">
              <img
                alt={user.user.username}
                src="https://cdn.discordapp.com/avatars/{user.user.id}/{user.user.avatar}.png?size=64"
              />
            </div>
          {:else}
            <div class="bg-neutral text-neutral-content w-10 rounded-full">
              <span>{user.user.username.charAt(0).toUpperCase()}</span>
            </div>
          {/if}
        </div>
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <ul tabindex="0" class="dropdown-content menu bg-base-300 rounded-box z-10 w-52 p-2 shadow">
          <li class="menu-title">{user.user.username}</li>
          <li><button onclick={logout}>Logout</button></li>
        </ul>
      </div>
    {/if}
  </div>
</div>
