<script lang="ts">
  import ToggleField from "../ui/ToggleField.svelte";
  import RoleSelect from "../ui/RoleSelect.svelte";
  import V2MessageToggle from "../ui/V2MessageToggle.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";
  import { guildDataStore } from "../../stores/guild-data.svelte";
  import { isDirty } from "../../utils/dirty";
  import { GatekeepSettingsSchema } from "../../../gen/heimdallr/v1/guild_settings_pb";

  const settings = settingsStore();
  const guildData = guildDataStore();
  const section = $derived(settings.gatekeep);
  const dirty = $derived(isDirty(GatekeepSettingsSchema, section.data, section.saved));

  $effect(() => { guildData.loadPlaceholders(); });

  const helpText = $derived(
    guildData.placeholders
      .map((p) => `${p.placeholder} — ${p.description}`)
      .join("\n")
  );
</script>

<div id="gatekeep" class="card bg-base-100 shadow-md">
  <div class="card-body">
    <h2 class="card-title">Gatekeep</h2>
    <p class="text-base-content/60 text-sm">
      Member verification system. New members get a pending role and must be approved to access the server.
    </p>
    <div class="divider"></div>

    {#if section.loading}
      <span class="loading loading-dots loading-md"></span>
    {:else}
      <ToggleField
        label="Enabled"
        description="Enable the gatekeep verification system"
        checked={section.data.enabled}
        onchange={(v) => (section.data.enabled = v)}
      />

      <RoleSelect
        label="Pending Role"
        description="Role assigned to unverified members"
        value={section.data.pendingRole}
        guildId={section.data.guildId}
        onchange={(v) => (section.data.pendingRole = v)}
      />

      <RoleSelect
        label="Approved Role"
        description="Role assigned to verified members"
        value={section.data.approvedRole}
        guildId={section.data.guildId}
        onchange={(v) => (section.data.approvedRole = v)}
      />

      <ToggleField
        label="Add pending role on join"
        description="Automatically assign the pending role when a new member joins"
        checked={section.data.addPendingRoleOnJoin}
        onchange={(v) => (section.data.addPendingRoleOnJoin = v)}
      />

      <V2MessageToggle
        label="Approved message"
        description="Message sent to the user when they are approved"
        {helpText}
        plainText={section.data.approvedMessage}
        v2Enabled={section.data.approvedMessageV2}
        v2Json={section.data.approvedMessageV2Json}
        onPlainTextChange={(v) => (section.data.approvedMessage = v)}
        onV2EnabledChange={(v) => (section.data.approvedMessageV2 = v)}
        onV2JsonChange={(v) => (section.data.approvedMessageV2Json = v)}
      />

      <SaveButton
        {dirty}
        saving={section.saving}
        error={section.error}
        onsave={() => settings.saveGatekeep()}
      />
    {/if}
  </div>
</div>
