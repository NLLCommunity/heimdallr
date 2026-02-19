<script lang="ts">
  import { create } from "@bufbuild/protobuf";
  import ChannelSelect from "../ui/ChannelSelect.svelte";
  import NumberField from "../ui/NumberField.svelte";
  import ToggleField from "../ui/ToggleField.svelte";
  import { settingsStore } from "../../stores/settings.svelte";
  import { isDirty } from "../../utils/dirty";
  import {
    type PaceControlChannel,
    PaceControlChannelSchema,
  } from "../../../gen/heimdallr/v1/guild_settings_pb";

  const settings = settingsStore();
  const section = $derived(settings.paceControl);

  // Get guild ID from another section that's always loaded
  const guildId = $derived(settings.modChannel.data.guildId);

  let adding = $state(false);
  let newChannel = $state<PaceControlChannel>(create(PaceControlChannelSchema));

  function resetNew() {
    newChannel = create(PaceControlChannelSchema);
    newChannel.enabled = true;
    newChannel.targetWpm = 100;
    newChannel.minSlowmode = 0;
    newChannel.maxSlowmode = 30;
    newChannel.activationWpm = 0;
    newChannel.wpmWindowSeconds = 60;
    newChannel.userWindowSeconds = 120;
    adding = false;
  }

  function startAdd() {
    resetNew();
    newChannel.guildId = guildId;
    adding = true;
  }

  async function saveNew() {
    newChannel.guildId = guildId;
    await settings.savePaceControlChannel(newChannel);
    resetNew();
  }

  async function saveExisting(ch: PaceControlChannel) {
    await settings.savePaceControlChannel(ch);
  }

  function isChannelDirty(ch: PaceControlChannel): boolean {
    const saved = section.savedChannels.find((s) => s.channelId === ch.channelId);
    if (!saved) return true;
    return isDirty(PaceControlChannelSchema, ch, saved);
  }

  async function deleteChannel(channelId: string) {
    await settings.deletePaceControlChannel(guildId, channelId);
  }
</script>

<div id="pace-control" class="card bg-base-100 shadow-md">
  <div class="card-body">
    <h2 class="card-title">Pace Control</h2>
    <p class="text-base-content/60 text-sm">
      Automatically adjust slow mode based on channel activity to keep
      conversations at a manageable pace.
    </p>
    <div class="divider"></div>

    {#if section.loading}
      <span class="loading loading-dots loading-md"></span>
    {:else}
      {#each section.channels as ch, i}
        <div class="border-base-300 rounded-lg border p-4">
          <div class="flex flex-col gap-3">
            <ChannelSelect
              label="Channel"
              value={ch.channelId}
              {guildId}
              onchange={(v) => (section.channels[i].channelId = v)}
            />

            <ToggleField
              label="Enabled"
              description="Enable pace control for this channel"
              checked={ch.enabled}
              onchange={(v) => (section.channels[i].enabled = v)}
            />

            <NumberField
              label="Target WPM"
              description="Target words per minute for the channel"
              value={ch.targetWpm}
              min={10}
              max={1000}
              onchange={(v) => (section.channels[i].targetWpm = v)}
            />

            <NumberField
              label="Activation WPM"
              description="WPM threshold before pace control kicks in (0 = always active)"
              value={ch.activationWpm}
              min={0}
              max={2000}
              onchange={(v) => (section.channels[i].activationWpm = v)}
            />

            <NumberField
              label="Min slowmode (seconds)"
              description="Minimum slow mode delay"
              value={ch.minSlowmode}
              min={0}
              max={120}
              onchange={(v) => (section.channels[i].minSlowmode = v)}
            />

            <NumberField
              label="Max slowmode (seconds)"
              description="Maximum slow mode delay"
              value={ch.maxSlowmode}
              min={1}
              max={120}
              onchange={(v) => (section.channels[i].maxSlowmode = v)}
            />

            <NumberField
              label="WPM window (seconds)"
              description="How far back to measure words per minute"
              value={ch.wpmWindowSeconds}
              min={10}
              max={300}
              onchange={(v) => (section.channels[i].wpmWindowSeconds = v)}
            />

            <NumberField
              label="User window (seconds)"
              description="How far back to count active users"
              value={ch.userWindowSeconds}
              min={10}
              max={300}
              onchange={(v) => (section.channels[i].userWindowSeconds = v)}
            />

            <div class="mt-2 flex items-center gap-2">
              <button
                class="btn btn-primary btn-sm"
                disabled={!isChannelDirty(ch) || section.saving}
                onclick={() => saveExisting(ch)}
              >
                {#if section.saving}
                  <span class="loading loading-spinner loading-xs"></span>
                {:else}
                  Save
                {/if}
              </button>
              <button
                class="btn btn-error btn-outline btn-sm"
                disabled={section.saving}
                onclick={() => deleteChannel(ch.channelId)}
              >
                Delete
              </button>
              {#if section.successChannelId === ch.channelId}
                <span class="text-success text-sm">Saved</span>
              {/if}
            </div>
          </div>
        </div>
      {/each}

      {#if adding}
        <div class="border-primary rounded-lg border border-dashed p-4">
          <div class="flex flex-col gap-3">
            <ChannelSelect
              label="Channel"
              value={newChannel.channelId}
              {guildId}
              onchange={(v) => (newChannel.channelId = v)}
            />

            <ToggleField
              label="Enabled"
              description="Enable pace control for this channel"
              checked={newChannel.enabled}
              onchange={(v) => (newChannel.enabled = v)}
            />

            <NumberField
              label="Target WPM"
              description="Target words per minute for the channel"
              value={newChannel.targetWpm}
              min={10}
              max={1000}
              onchange={(v) => (newChannel.targetWpm = v)}
            />

            <NumberField
              label="Activation WPM"
              description="WPM threshold before pace control kicks in (0 = always active)"
              value={newChannel.activationWpm}
              min={0}
              max={2000}
              onchange={(v) => (newChannel.activationWpm = v)}
            />

            <NumberField
              label="Min slowmode (seconds)"
              description="Minimum slow mode delay"
              value={newChannel.minSlowmode}
              min={0}
              max={120}
              onchange={(v) => (newChannel.minSlowmode = v)}
            />

            <NumberField
              label="Max slowmode (seconds)"
              description="Maximum slow mode delay"
              value={newChannel.maxSlowmode}
              min={1}
              max={120}
              onchange={(v) => (newChannel.maxSlowmode = v)}
            />

            <NumberField
              label="WPM window (seconds)"
              description="How far back to measure words per minute"
              value={newChannel.wpmWindowSeconds}
              min={10}
              max={300}
              onchange={(v) => (newChannel.wpmWindowSeconds = v)}
            />

            <NumberField
              label="User window (seconds)"
              description="How far back to count active users"
              value={newChannel.userWindowSeconds}
              min={10}
              max={300}
              onchange={(v) => (newChannel.userWindowSeconds = v)}
            />

            <div class="mt-2 flex gap-2">
              <button
                class="btn btn-primary btn-sm"
                disabled={section.saving || !newChannel.channelId}
                onclick={saveNew}
              >
                {#if section.saving}
                  <span class="loading loading-spinner loading-xs"></span>
                {:else}
                  Add
                {/if}
              </button>
              <button
                class="btn btn-ghost btn-sm"
                onclick={resetNew}
              >
                Cancel
              </button>
            </div>
          </div>
        </div>
      {:else}
        <button class="btn btn-outline btn-sm w-fit" onclick={startAdd}>
          + Add channel
        </button>
      {/if}

      {#if section.error}
        <span class="text-error text-sm">{section.error}</span>
      {/if}
    {/if}
  </div>
</div>
