<script lang="ts">
  import ToggleField from "../ui/ToggleField.svelte";
  import SnowflakeField from "../ui/SnowflakeField.svelte";
  import TextareaField from "../ui/TextareaField.svelte";
  import SaveButton from "../ui/SaveButton.svelte";
  import { settingsStore } from "../../stores/settings.svelte";

  const settings = settingsStore();
  const section = $derived(settings.gatekeep);
  const dirty = $derived(
    section.data.enabled !== section.saved.enabled ||
    section.data.pendingRole !== section.saved.pendingRole ||
    section.data.approvedRole !== section.saved.approvedRole ||
    section.data.addPendingRoleOnJoin !== section.saved.addPendingRoleOnJoin ||
    section.data.approvedMessage !== section.saved.approvedMessage
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

      <SnowflakeField
        label="Pending Role"
        description="Role assigned to unverified members"
        value={section.data.pendingRole}
        placeholder="Role ID"
        onchange={(v) => (section.data.pendingRole = v)}
      />

      <SnowflakeField
        label="Approved Role"
        description="Role assigned to verified members"
        value={section.data.approvedRole}
        placeholder="Role ID"
        onchange={(v) => (section.data.approvedRole = v)}
      />

      <ToggleField
        label="Add pending role on join"
        description="Automatically assign the pending role when a new member joins"
        checked={section.data.addPendingRoleOnJoin}
        onchange={(v) => (section.data.addPendingRoleOnJoin = v)}
      />

      <TextareaField
        label="Approved message"
        description="Message sent to the user when they are approved"
        value={section.data.approvedMessage}
        onchange={(v) => (section.data.approvedMessage = v)}
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
