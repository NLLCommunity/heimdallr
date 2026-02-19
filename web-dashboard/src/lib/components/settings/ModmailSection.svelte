<script lang="ts">
  import ChannelSelect from "../ui/ChannelSelect.svelte";
  import RoleSelect from "../ui/RoleSelect.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";
  import { isDirty } from "../../utils/dirty";
  import { ModmailSettingsSchema } from "../../../gen/heimdallr/v1/guild_settings_pb";

  const settings = settingsStore();
  const section = $derived(settings.modmail);
  const dirty = $derived(isDirty(ModmailSettingsSchema, section.data, section.saved));
</script>

<div id="modmail" class="card bg-base-100 shadow-md">
  <div class="card-body">
    <h2 class="card-title">Modmail</h2>
    <p class="text-base-content/60 text-sm">
      Configure channels and roles for the modmail report system.
    </p>
    <div class="divider"></div>

    {#if section.loading}
      <span class="loading loading-dots loading-md"></span>
    {:else}
      <ChannelSelect
        label="Report threads channel"
        description="Channel where report threads are created"
        value={section.data.reportThreadsChannel}
        guildId={section.data.guildId}
        onchange={(v) => (section.data.reportThreadsChannel = v)}
      />

      <ChannelSelect
        label="Report notification channel"
        description="Channel where new report notifications are posted"
        value={section.data.reportNotificationChannel}
        guildId={section.data.guildId}
        onchange={(v) => (section.data.reportNotificationChannel = v)}
      />

      <RoleSelect
        label="Report ping role"
        description="Role pinged when a new report is submitted"
        value={section.data.reportPingRole}
        guildId={section.data.guildId}
        onchange={(v) => (section.data.reportPingRole = v)}
      />

      <SaveButton
        {dirty}
        saving={section.saving}
        success={section.success}
        error={section.error}
        onsave={() => settings.saveModmail()}
      />
    {/if}
  </div>
</div>
