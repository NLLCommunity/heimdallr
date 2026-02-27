<script lang="ts">
  import Navbar from "./Navbar.svelte";
  import Sidebar from "./Sidebar.svelte";
  import type { Snippet } from "svelte";

  interface Props {
    currentGuildId?: string;
    showSidebar?: boolean;
    children: Snippet;
  }

  let { currentGuildId, showSidebar = false, children }: Props = $props();
</script>

<div class="flex min-h-screen flex-col">
  <Navbar {currentGuildId} />
  <div class="flex flex-1">
    {#if showSidebar}
      <Sidebar />
    {/if}
    <main class="flex-1 p-6">
      {@render children()}
    </main>
  </div>
</div>
