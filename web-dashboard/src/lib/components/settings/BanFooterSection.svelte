<script lang="ts">
  import ToggleField from "../ui/ToggleField.svelte";
  import TextareaField from "../ui/TextareaField.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";
  import { isDirty } from "../../utils/dirty";
  import { BanFooterSettingsSchema } from "../../../gen/heimdallr/v1/guild_settings_pb";

  const settings = settingsStore();
  const section = $derived(settings.banFooter);
  const dirty = $derived(isDirty(BanFooterSettingsSchema, section.data, section.saved));
</script>

<div id="ban-footer" class="card bg-base-100 shadow-md">
  <div class="card-body">
    <h2 class="card-title">Ban Footer</h2>
    <p class="text-base-content/60 text-sm">
      Additional text appended to ban notification messages sent to users.
    </p>
    <div class="divider"></div>

    {#if section.loading}
      <span class="loading loading-dots loading-md"></span>
    {:else}
      <TextareaField
        label="Footer text"
        description="Text appended to ban DM messages (e.g., appeal information)"
        value={section.data.footer}
        onchange={(v) => (section.data.footer = v)}
      />

      <ToggleField
        label="Always send footer"
        description="Send the footer even when no custom reason is provided"
        checked={section.data.alwaysSend}
        onchange={(v) => (section.data.alwaysSend = v)}
      />

      <SaveButton
        {dirty}
        saving={section.saving}
        error={section.error}
        onsave={() => settings.saveBanFooter()}
      />
    {/if}
  </div>
</div>
