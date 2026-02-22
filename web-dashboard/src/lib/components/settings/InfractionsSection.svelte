<script lang="ts">
  import NumberField from "../ui/NumberField.svelte";
  import ToggleField from "../ui/ToggleField.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";
  import { isDirty } from "../../utils/dirty";
  import { InfractionSettingsSchema } from "../../../gen/heimdallr/v1/guild_settings_pb";

  const settings = settingsStore();
  const section = $derived(settings.infractions);
  const dirty = $derived(isDirty(InfractionSettingsSchema, section.data, section.saved));
</script>

<div id="infractions" class="card bg-base-100 shadow-md">
  <div class="card-body">
    <h2 class="card-title">Infractions</h2>
    <p class="text-base-content/60 text-sm">
      Configure infraction severity decay and join notifications for warned users.
    </p>
    <div class="divider"></div>

    {#if section.loading}
      <span class="loading loading-dots loading-md"></span>
    {:else}
      <NumberField
        label="Half-life (days)"
        description="How many days until an infraction's weight decays to half"
        value={section.data.halfLifeDays}
        min={0}
        step={0.5}
        onchange={(v) => (section.data.halfLifeDays = v)}
      />

      <ToggleField
        label="Notify on warned user join"
        description="Post a notification when a user with warnings joins the server"
        checked={section.data.notifyOnWarnedUserJoin}
        onchange={(v) => (section.data.notifyOnWarnedUserJoin = v)}
      />

      <NumberField
        label="Notify severity threshold"
        description="Minimum infraction severity to trigger a join notification"
        value={section.data.notifyWarnSeverityThreshold}
        min={0}
        step={0.1}
        onchange={(v) => (section.data.notifyWarnSeverityThreshold = v)}
      />

      <SaveButton
        {dirty}
        saving={section.saving}
        error={section.error}
        onsave={() => settings.saveInfractions()}
      />
    {/if}
  </div>
</div>
