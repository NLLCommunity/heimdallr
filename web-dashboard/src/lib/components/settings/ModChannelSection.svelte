<script lang="ts">
  import ChannelSelect from "../ui/ChannelSelect.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";

  const settings = settingsStore();
  const section = $derived(settings.modChannel);
  const dirty = $derived(
    section.data.moderatorChannel !== section.saved.moderatorChannel
  );
</script>

<div id="mod-channel" class="card bg-base-100 shadow-md">
  <div class="card-body">
    <h2 class="card-title">Moderator Channel</h2>
    <p class="text-base-content/60 text-sm">
      The channel where bot notifications and moderator information are sent.
    </p>
    <div class="divider"></div>

    {#if section.loading}
      <span class="loading loading-dots loading-md"></span>
    {:else}
      <ChannelSelect
        label="Channel"
        description="The Discord channel for moderator notifications"
        value={section.data.moderatorChannel}
        guildId={section.data.guildId}
        onchange={(v) => (section.data.moderatorChannel = v)}
      />
      <SaveButton
        {dirty}
        saving={section.saving}
        error={section.error}
        onsave={() => settings.saveModChannel()}
      />
    {/if}
  </div>
</div>
