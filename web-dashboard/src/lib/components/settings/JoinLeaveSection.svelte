<script lang="ts">
  import ToggleField from "../ui/ToggleField.svelte";
  import SnowflakeField from "../ui/SnowflakeField.svelte";
  import TextareaField from "../ui/TextareaField.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";

  const settings = settingsStore();
  const section = $derived(settings.joinLeave);
  const dirty = $derived(
    section.data.joinMessageEnabled !== section.saved.joinMessageEnabled ||
    section.data.joinMessage !== section.saved.joinMessage ||
    section.data.leaveMessageEnabled !== section.saved.leaveMessageEnabled ||
    section.data.leaveMessage !== section.saved.leaveMessage ||
    section.data.channel !== section.saved.channel
  );

  const messagePlaceholders = `{user} — The user's mention
{username} — The user's name
{server} — The server name
{member_count} — Current member count`;
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
      <SnowflakeField
        label="Channel"
        description="Channel where join/leave messages are sent"
        value={section.data.channel}
        placeholder="Channel ID"
        onchange={(v) => (section.data.channel = v)}
      />

      <div class="divider text-sm">Join Message</div>

      <ToggleField
        label="Join message enabled"
        checked={section.data.joinMessageEnabled}
        onchange={(v) => (section.data.joinMessageEnabled = v)}
      />

      <TextareaField
        label="Join message"
        value={section.data.joinMessage}
        helpText={messagePlaceholders}
        onchange={(v) => (section.data.joinMessage = v)}
      />

      <div class="divider text-sm">Leave Message</div>

      <ToggleField
        label="Leave message enabled"
        checked={section.data.leaveMessageEnabled}
        onchange={(v) => (section.data.leaveMessageEnabled = v)}
      />

      <TextareaField
        label="Leave message"
        value={section.data.leaveMessage}
        helpText={messagePlaceholders}
        onchange={(v) => (section.data.leaveMessage = v)}
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
