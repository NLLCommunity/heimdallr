<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import { isLoggedIn } from "../auth/auth";
  import { authClient } from "../api/client";
  import { userStore } from "../stores/user.svelte";
  import { guildsStore } from "../stores/guilds.svelte";
  import { settingsStore } from "../stores/settings.svelte";
  import Layout from "../components/layout/Layout.svelte";
  import ModChannelSection from "../components/settings/ModChannelSection.svelte";
  import InfractionsSection from "../components/settings/InfractionsSection.svelte";
  import GatekeepSection from "../components/settings/GatekeepSection.svelte";
  import JoinLeaveSection from "../components/settings/JoinLeaveSection.svelte";
  import AntiSpamSection from "../components/settings/AntiSpamSection.svelte";
  import BanFooterSection from "../components/settings/BanFooterSection.svelte";
  import ModmailSection from "../components/settings/ModmailSection.svelte";

  interface Props {
    params: { id: string };
  }

  let { params }: Props = $props();

  const user = userStore();
  const guilds = guildsStore();
  const settings = settingsStore();

  let loading = $state(true);

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

    if (guilds.guilds.length === 0) {
      try {
        const res = await authClient.listGuilds({});
        guilds.guilds = res.guilds;
      } catch {
        // Continue loading settings even if guild list fails
      }
    }

    await settings.loadAll(params.id);
    loading = false;
  });
</script>

<Layout currentGuildId={params.id} showSidebar={true}>
  {#if loading}
    <div class="flex justify-center py-12">
      <span class="loading loading-spinner loading-lg"></span>
    </div>
  {:else}
    <div class="mx-auto flex max-w-3xl flex-col gap-6">
      <ModChannelSection />
      <InfractionsSection />
      <GatekeepSection />
      <JoinLeaveSection />
      <AntiSpamSection />
      <BanFooterSection />
      <ModmailSection />
    </div>
  {/if}
</Layout>
