<script lang="ts">
  import { guildDataStore } from "../../stores/guild-data.svelte";

  // Discord channel types
  const CHANNEL_TYPE_TEXT = 0;
  const CHANNEL_TYPE_VOICE = 2;
  const CHANNEL_TYPE_CATEGORY = 4;
  const CHANNEL_TYPE_NEWS = 5;
  const CHANNEL_TYPE_STAGE = 13;
  const CHANNEL_TYPE_FORUM = 15;
  const CHANNEL_TYPE_MEDIA = 16;

  const TEXT_CHANNEL_TYPES = [CHANNEL_TYPE_TEXT, CHANNEL_TYPE_NEWS];

  interface Props {
    label: string;
    description?: string;
    value: string;
    guildId: string;
    allowedTypes?: number[];
    onchange: (value: string) => void;
  }

  let {
    label,
    description,
    value,
    guildId,
    allowedTypes = TEXT_CHANNEL_TYPES,
    onchange,
  }: Props = $props();

  const guildData = guildDataStore();

  $effect(() => {
    if (guildId) {
      guildData.load(guildId);
    }
  });

  interface ChannelGroup {
    categoryName: string;
    categoryPosition: number;
    channels: { id: string; name: string; type: number; position: number }[];
  }

  const grouped = $derived.by(() => {
    const channels = guildData.channels;
    const categories = channels.filter((c) => c.type === CHANNEL_TYPE_CATEGORY);
    const categoryMap = new Map(categories.map((c) => [c.id, c]));

    const selectable = channels.filter(
      (c) => c.type !== CHANNEL_TYPE_CATEGORY && allowedTypes.includes(c.type)
    );

    const groups = new Map<string, ChannelGroup>();

    // Uncategorized group
    groups.set("", {
      categoryName: "",
      categoryPosition: -1,
      channels: [],
    });

    for (const cat of categories) {
      groups.set(cat.id, {
        categoryName: cat.name,
        categoryPosition: cat.position,
        channels: [],
      });
    }

    for (const ch of selectable) {
      const parentId = ch.parentId || "";
      let group = groups.get(parentId);
      if (!group) {
        group = groups.get("")!;
      }
      group.channels.push({
        id: ch.id,
        name: ch.name,
        type: ch.type,
        position: ch.position,
      });
    }

    // Sort channels within each group by position
    for (const group of groups.values()) {
      group.channels.sort((a, b) => a.position - b.position);
    }

    // Return groups sorted by category position, uncategorized first
    return [...groups.values()]
      .filter((g) => g.channels.length > 0)
      .sort((a, b) => a.categoryPosition - b.categoryPosition);
  });

  function channelPrefix(type: number): string {
    switch (type) {
      case CHANNEL_TYPE_NEWS: return "\u{1F4E2} ";
      default: return "# ";
    }
  }
</script>

<label class="flex items-center justify-between gap-4">
  <div class="flex flex-col">
    <span class="text-sm font-semibold">{label}</span>
    {#if description}
      <span class="text-base-content/60 text-xs">{description}</span>
    {/if}
  </div>
  {#if guildData.loading}
    <span class="loading loading-dots loading-sm"></span>
  {:else}
    <select
      class="select select-bordered w-full max-w-xs shrink-0"
      {value}
      onchange={(e) => onchange(e.currentTarget.value)}
    >
      <option value="">None</option>
      {#each grouped as group}
        {#if group.categoryName}
          <optgroup label={group.categoryName}>
            {#each group.channels as ch}
              <option value={ch.id}>{channelPrefix(ch.type)}{ch.name}</option>
            {/each}
          </optgroup>
        {:else}
          {#each group.channels as ch}
            <option value={ch.id}>{channelPrefix(ch.type)}{ch.name}</option>
          {/each}
        {/if}
      {/each}
    </select>
  {/if}
</label>
