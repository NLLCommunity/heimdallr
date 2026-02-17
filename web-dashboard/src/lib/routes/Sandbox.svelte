<script lang="ts">
  import { onMount } from "svelte";
  import { push } from "svelte-spa-router";
  import { isLoggedIn } from "../auth/auth";
  import { authClient, settingsClient } from "../api/client";
  import { userStore } from "../stores/user.svelte";
  import { guildsStore } from "../stores/guilds.svelte";
  import Layout from "../components/layout/Layout.svelte";
  import MessageBuilder from "../components/sandbox/MessageBuilder.svelte";
  import MessagePreview from "../components/sandbox/MessagePreview.svelte";
  import ChannelSelect from "../components/ui/ChannelSelect.svelte";
  import { serializeComponents, type ComponentNode } from "../components/sandbox/serialize";

  interface Props {
    params: { id: string };
  }

  let { params }: Props = $props();

  const user = userStore();
  const guilds = guildsStore();

  let loading = $state(true);
  let components = $state<ComponentNode[]>([]);
  let channelId = $state("");
  let sending = $state(false);
  let statusMessage = $state("");
  let statusError = $state(false);

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
        // continue
      }
    }

    loading = false;
  });

  async function sendMessage() {
    if (!channelId) {
      statusMessage = "Please select a channel";
      statusError = true;
      return;
    }

    const json = serializeComponents(components);
    const parsed = JSON.parse(json);
    if (parsed.length === 0) {
      statusMessage = "Add at least one component with content";
      statusError = true;
      return;
    }

    sending = true;
    statusMessage = "";

    try {
      const res = await settingsClient.sendComponentsMessage({
        guildId: params.id,
        channelId,
        componentsJson: json,
      });
      statusMessage = `Message sent! ID: ${res.messageId}`;
      statusError = false;
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Unknown error";
      statusMessage = `Failed: ${message}`;
      statusError = true;
    } finally {
      sending = false;
    }
  }
</script>

<Layout currentGuildId={params.id}>
  {#if loading}
    <div class="flex justify-center py-12">
      <span class="loading loading-spinner loading-lg"></span>
    </div>
  {:else}
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-6 lg:flex-row">
      <!-- Builder -->
      <div class="flex-1">
        <h2 class="mb-4 text-lg font-bold">Components V2 Builder</h2>
        <MessageBuilder bind:components />
      </div>

      <!-- Preview + Send -->
      <div class="flex w-full flex-col gap-4 lg:w-96">
        <MessagePreview {components} />

        <div class="card bg-base-100 border-base-300 border">
          <div class="card-body gap-3 p-4">
            <h3 class="text-sm font-semibold opacity-60">Send</h3>
            <ChannelSelect
              label="Target Channel"
              value={channelId}
              guildId={params.id}
              onchange={(v) => (channelId = v)}
            />
            <button
              class="btn btn-primary btn-sm"
              onclick={sendMessage}
              disabled={sending}
            >
              {#if sending}
                <span class="loading loading-spinner loading-xs"></span>
              {/if}
              Send Message
            </button>
            {#if statusMessage}
              <div class="text-sm" class:text-error={statusError} class:text-success={!statusError}>
                {statusMessage}
              </div>
            {/if}
          </div>
        </div>
      </div>
    </div>
  {/if}
</Layout>
