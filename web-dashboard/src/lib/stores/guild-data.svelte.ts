import { settingsClient } from "../api/client";
import type { Channel, Role, TemplatePlaceholder } from "../../gen/heimdallr/v1/guild_settings_pb";

let channels = $state<Channel[]>([]);
let roles = $state<Role[]>([]);
let loading = $state(false);
let loadedGuildId = "";
let pending: Promise<void> | null = null;

let placeholders = $state<TemplatePlaceholder[]>([]);
let placeholdersLoaded = false;
let placeholdersPending: Promise<void> | null = null;

async function fetchGuildData(guildId: string) {
  loading = true;
  try {
    const [chRes, roleRes] = await Promise.all([
      settingsClient.listChannels({ guildId }),
      settingsClient.listRoles({ guildId }),
    ]);
    channels = chRes.channels;
    roles = roleRes.roles;
    loadedGuildId = guildId;
  } finally {
    loading = false;
    pending = null;
  }
}

async function fetchPlaceholders() {
  try {
    const res = await settingsClient.getTemplatePlaceholders({});
    placeholders = res.placeholders;
    placeholdersLoaded = true;
  } finally {
    placeholdersPending = null;
  }
}

export function guildDataStore() {
  return {
    get channels() { return channels; },
    get roles() { return roles; },
    get loading() { return loading; },
    get placeholders() { return placeholders; },

    load(guildId: string): Promise<void> {
      if (guildId === loadedGuildId) return Promise.resolve();
      if (pending) return pending;
      pending = fetchGuildData(guildId);
      return pending;
    },

    loadPlaceholders(): Promise<void> {
      if (placeholdersLoaded) return Promise.resolve();
      if (placeholdersPending) return placeholdersPending;
      placeholdersPending = fetchPlaceholders();
      return placeholdersPending;
    },
  };
}
