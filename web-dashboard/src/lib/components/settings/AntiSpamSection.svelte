<script lang="ts">
  import ToggleField from "../ui/ToggleField.svelte";
  import NumberField from "../ui/NumberField.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";
  import { isDirty } from "../../utils/dirty";
  import { AntiSpamSettingsSchema } from "../../../gen/heimdallr/v1/guild_settings_pb";

  const settings = settingsStore();
  const section = $derived(settings.antiSpam);
  const dirty = $derived(isDirty(AntiSpamSettingsSchema, section.data, section.saved));
</script>

<div id="anti-spam" class="card bg-base-100 shadow-md">
  <div class="card-body">
    <h2 class="card-title">Anti-Spam</h2>
    <p class="text-base-content/60 text-sm">
      Automatically detect and act on spam messages.
    </p>
    <div class="divider"></div>

    {#if section.loading}
      <span class="loading loading-dots loading-md"></span>
    {:else}
      <ToggleField
        label="Enabled"
        description="Enable anti-spam detection"
        checked={section.data.enabled}
        onchange={(v) => (section.data.enabled = v)}
      />

      <NumberField
        label="Message count"
        description="Number of messages within the cooldown period to trigger anti-spam"
        value={section.data.count}
        min={2}
        max={10}
        onchange={(v) => (section.data.count = v)}
      />

      <NumberField
        label="Cooldown (seconds)"
        description="Time window in seconds for counting messages"
        value={section.data.cooldownSeconds}
        min={1}
        max={60}
        onchange={(v) => (section.data.cooldownSeconds = v)}
      />

      <SaveButton
        {dirty}
        saving={section.saving}
        error={section.error}
        onsave={() => settings.saveAntiSpam()}
      />
    {/if}
  </div>
</div>
