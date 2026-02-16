<script lang="ts">
  import SnowflakeField from "../ui/SnowflakeField.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";

  const settings = settingsStore();
  const section = $derived(settings.modmail);
  const dirty = $derived(
    section.data.reportThreadsChannel !== section.saved.reportThreadsChannel ||
    section.data.reportNotificationChannel !== section.saved.reportNotificationChannel ||
    section.data.reportPingRole !== section.saved.reportPingRole
  );
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
      <SnowflakeField
        label="Report threads channel"
        description="Channel where report threads are created"
        value={section.data.reportThreadsChannel}
        placeholder="Channel ID"
        onchange={(v) => (section.data.reportThreadsChannel = v)}
      />

      <SnowflakeField
        label="Report notification channel"
        description="Channel where new report notifications are posted"
        value={section.data.reportNotificationChannel}
        placeholder="Channel ID"
        onchange={(v) => (section.data.reportNotificationChannel = v)}
      />

      <SnowflakeField
        label="Report ping role"
        description="Role pinged when a new report is submitted"
        value={section.data.reportPingRole}
        placeholder="Role ID"
        onchange={(v) => (section.data.reportPingRole = v)}
      />

      <SaveButton
        {dirty}
        saving={section.saving}
        error={section.error}
        onsave={() => settings.saveModmail()}
      />
    {/if}
  </div>
</div>
