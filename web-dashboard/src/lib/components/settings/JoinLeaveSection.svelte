<script lang="ts">
  import ToggleField from "../ui/ToggleField.svelte";
  import ChannelSelect from "../ui/ChannelSelect.svelte";
  import V2MessageToggle from "../ui/V2MessageToggle.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";
  import { guildDataStore } from "../../stores/guild-data.svelte";

  const settings = settingsStore();
  const guildData = guildDataStore();
  const section = $derived(settings.joinLeave);
  const dirty = $derived(
    section.data.joinMessageEnabled !== section.saved.joinMessageEnabled ||
    section.data.joinMessage !== section.saved.joinMessage ||
    section.data.joinMessageV2 !== section.saved.joinMessageV2 ||
    section.data.joinMessageV2Json !== section.saved.joinMessageV2Json ||
    section.data.leaveMessageEnabled !== section.saved.leaveMessageEnabled ||
    section.data.leaveMessage !== section.saved.leaveMessage ||
    section.data.leaveMessageV2 !== section.saved.leaveMessageV2 ||
    section.data.leaveMessageV2Json !== section.saved.leaveMessageV2Json ||
    section.data.channel !== section.saved.channel
  );

  $effect(() => { guildData.loadPlaceholders(); });

  const helpText = $derived(
    guildData.placeholders
      .map((p) => `${p.placeholder} — ${p.description}`)
      .join("\n")
  );
</script>

<div id="join-leave" class="card bg-base-100 shadow-md">
  <div class="card-body">
    <h2 class="card-title">Join/Leave Messages</h2>
    <p class="text-base-content/60 text-sm">
      Configure messages sent when members join or leave the server.
    </p>
    <div class="divider"></div>

    {#if section.loading}
      <span class="loading loading-dots loading-md"></span>
    {:else}
      <ChannelSelect
        label="Channel"
        description="Channel where join/leave messages are sent"
        value={section.data.channel}
        guildId={section.data.guildId}
        onchange={(v) => (section.data.channel = v)}
      />

      <div class="divider text-sm">Join Message</div>

      <ToggleField
        label="Join message enabled"
        checked={section.data.joinMessageEnabled}
        onchange={(v) => (section.data.joinMessageEnabled = v)}
      />

      <V2MessageToggle
        label="Join message"
        {helpText}
        plainText={section.data.joinMessage}
        v2Enabled={section.data.joinMessageV2}
        v2Json={section.data.joinMessageV2Json}
        onPlainTextChange={(v) => (section.data.joinMessage = v)}
        onV2EnabledChange={(v) => (section.data.joinMessageV2 = v)}
        onV2JsonChange={(v) => (section.data.joinMessageV2Json = v)}
      />

      <div class="divider text-sm">Leave Message</div>

      <ToggleField
        label="Leave message enabled"
        checked={section.data.leaveMessageEnabled}
        onchange={(v) => (section.data.leaveMessageEnabled = v)}
      />

      <V2MessageToggle
        label="Leave message"
        {helpText}
        plainText={section.data.leaveMessage}
        v2Enabled={section.data.leaveMessageV2}
        v2Json={section.data.leaveMessageV2Json}
        onPlainTextChange={(v) => (section.data.leaveMessage = v)}
        onV2EnabledChange={(v) => (section.data.leaveMessageV2 = v)}
        onV2JsonChange={(v) => (section.data.leaveMessageV2Json = v)}
      />

      <SaveButton
        {dirty}
        saving={section.saving}
        error={section.error}
        onsave={() => settings.saveJoinLeave()}
      />
    {/if}
  </div>
</div>
