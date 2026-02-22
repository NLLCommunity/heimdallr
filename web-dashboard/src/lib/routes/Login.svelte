<script lang="ts">
  import { push } from "svelte-spa-router";
  import { onMount } from "svelte";
  import { userStore } from "../stores/user.svelte";

  const user = userStore();

  onMount(() => {
    if (user.isLoggedIn) {
      push("/guilds");
    }
  });
</script>

<div class="hero min-h-screen bg-base-200">
  <div class="hero-content text-center">
    <div class="max-w-md">
      <h1 class="text-5xl font-bold">Heimdallr</h1>
      <p class="py-6">
        Manage your Discord server settings from the web.
      </p>
      <div class="card bg-base-100 shadow-xl">
        <div class="card-body">
          <h2 class="card-title justify-center">How to log in</h2>
          <p>
            Use the <code class="kbd kbd-sm">/admin-dashboard</code> command in a
            Discord server where Heimdallr is installed to get a login link.
          </p>
        </div>
      </div>
    </div>
  </div>
</div>
