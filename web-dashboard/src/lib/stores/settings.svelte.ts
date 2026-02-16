import { create } from "@bufbuild/protobuf";
import { settingsClient } from "../api/client";
import {
  type ModChannelSettings,
  ModChannelSettingsSchema,
  type InfractionSettings,
  InfractionSettingsSchema,
  type GatekeepSettings,
  GatekeepSettingsSchema,
  type JoinLeaveSettings,
  JoinLeaveSettingsSchema,
  type AntiSpamSettings,
  AntiSpamSettingsSchema,
  type BanFooterSettings,
  BanFooterSettingsSchema,
  type ModmailSettings,
  ModmailSettingsSchema,
} from "../../gen/heimdallr/v1/guild_settings_pb";

interface SectionState<T> {
  data: T;
  saved: T;
  saving: boolean;
  loading: boolean;
  error: string | null;
}

let modChannel = $state<SectionState<ModChannelSettings>>(makeDefault(ModChannelSettingsSchema));
let infractions = $state<SectionState<InfractionSettings>>(makeDefault(InfractionSettingsSchema));
let gatekeep = $state<SectionState<GatekeepSettings>>(makeDefault(GatekeepSettingsSchema));
let joinLeave = $state<SectionState<JoinLeaveSettings>>(makeDefault(JoinLeaveSettingsSchema));
let antiSpam = $state<SectionState<AntiSpamSettings>>(makeDefault(AntiSpamSettingsSchema));
let banFooter = $state<SectionState<BanFooterSettings>>(makeDefault(BanFooterSettingsSchema));
let modmail = $state<SectionState<ModmailSettings>>(makeDefault(ModmailSettingsSchema));

function makeDefault<T>(schema: Parameters<typeof create>[0]): SectionState<T> {
  return {
    data: create(schema) as T,
    saved: create(schema) as T,
    saving: false,
    loading: false,
    error: null,
  };
}

function clone<T>(schema: Parameters<typeof create>[0], obj: T): T {
  return create(schema, obj as Record<string, unknown>) as T;
}

export function settingsStore() {
  return {
    get modChannel() { return modChannel; },
    get infractions() { return infractions; },
    get gatekeep() { return gatekeep; },
    get joinLeave() { return joinLeave; },
    get antiSpam() { return antiSpam; },
    get banFooter() { return banFooter; },
    get modmail() { return modmail; },

    async loadAll(guildId: string) {
      await Promise.all([
        this.loadModChannel(guildId),
        this.loadInfractions(guildId),
        this.loadGatekeep(guildId),
        this.loadJoinLeave(guildId),
        this.loadAntiSpam(guildId),
        this.loadBanFooter(guildId),
        this.loadModmail(guildId),
      ]);
    },

    async loadModChannel(guildId: string) {
      modChannel.loading = true;
      modChannel.error = null;
      try {
        const res = await settingsClient.getModChannel({ guildId });
        modChannel.data = res;
        modChannel.saved = clone(ModChannelSettingsSchema, res);
      } catch (e: any) {
        modChannel.error = e.message;
      } finally {
        modChannel.loading = false;
      }
    },

    async saveModChannel() {
      modChannel.saving = true;
      modChannel.error = null;
      try {
        const res = await settingsClient.updateModChannel({ settings: modChannel.data });
        modChannel.data = res;
        modChannel.saved = clone(ModChannelSettingsSchema, res);
      } catch (e: any) {
        modChannel.error = e.message;
      } finally {
        modChannel.saving = false;
      }
    },

    async loadInfractions(guildId: string) {
      infractions.loading = true;
      infractions.error = null;
      try {
        const res = await settingsClient.getInfractionSettings({ guildId });
        infractions.data = res;
        infractions.saved = clone(InfractionSettingsSchema, res);
      } catch (e: any) {
        infractions.error = e.message;
      } finally {
        infractions.loading = false;
      }
    },

    async saveInfractions() {
      infractions.saving = true;
      infractions.error = null;
      try {
        const res = await settingsClient.updateInfractionSettings({ settings: infractions.data });
        infractions.data = res;
        infractions.saved = clone(InfractionSettingsSchema, res);
      } catch (e: any) {
        infractions.error = e.message;
      } finally {
        infractions.saving = false;
      }
    },

    async loadGatekeep(guildId: string) {
      gatekeep.loading = true;
      gatekeep.error = null;
      try {
        const res = await settingsClient.getGatekeepSettings({ guildId });
        gatekeep.data = res;
        gatekeep.saved = clone(GatekeepSettingsSchema, res);
      } catch (e: any) {
        gatekeep.error = e.message;
      } finally {
        gatekeep.loading = false;
      }
    },

    async saveGatekeep() {
      gatekeep.saving = true;
      gatekeep.error = null;
      try {
        const res = await settingsClient.updateGatekeepSettings({ settings: gatekeep.data });
        gatekeep.data = res;
        gatekeep.saved = clone(GatekeepSettingsSchema, res);
      } catch (e: any) {
        gatekeep.error = e.message;
      } finally {
        gatekeep.saving = false;
      }
    },

    async loadJoinLeave(guildId: string) {
      joinLeave.loading = true;
      joinLeave.error = null;
      try {
        const res = await settingsClient.getJoinLeaveSettings({ guildId });
        joinLeave.data = res;
        joinLeave.saved = clone(JoinLeaveSettingsSchema, res);
      } catch (e: any) {
        joinLeave.error = e.message;
      } finally {
        joinLeave.loading = false;
      }
    },

    async saveJoinLeave() {
      joinLeave.saving = true;
      joinLeave.error = null;
      try {
        const res = await settingsClient.updateJoinLeaveSettings({ settings: joinLeave.data });
        joinLeave.data = res;
        joinLeave.saved = clone(JoinLeaveSettingsSchema, res);
      } catch (e: any) {
        joinLeave.error = e.message;
      } finally {
        joinLeave.saving = false;
      }
    },

    async loadAntiSpam(guildId: string) {
      antiSpam.loading = true;
      antiSpam.error = null;
      try {
        const res = await settingsClient.getAntiSpamSettings({ guildId });
        antiSpam.data = res;
        antiSpam.saved = clone(AntiSpamSettingsSchema, res);
      } catch (e: any) {
        antiSpam.error = e.message;
      } finally {
        antiSpam.loading = false;
      }
    },

    async saveAntiSpam() {
      antiSpam.saving = true;
      antiSpam.error = null;
      try {
        const res = await settingsClient.updateAntiSpamSettings({ settings: antiSpam.data });
        antiSpam.data = res;
        antiSpam.saved = clone(AntiSpamSettingsSchema, res);
      } catch (e: any) {
        antiSpam.error = e.message;
      } finally {
        antiSpam.saving = false;
      }
    },

    async loadBanFooter(guildId: string) {
      banFooter.loading = true;
      banFooter.error = null;
      try {
        const res = await settingsClient.getBanFooterSettings({ guildId });
        banFooter.data = res;
        banFooter.saved = clone(BanFooterSettingsSchema, res);
      } catch (e: any) {
        banFooter.error = e.message;
      } finally {
        banFooter.loading = false;
      }
    },

    async saveBanFooter() {
      banFooter.saving = true;
      banFooter.error = null;
      try {
        const res = await settingsClient.updateBanFooterSettings({ settings: banFooter.data });
        banFooter.data = res;
        banFooter.saved = clone(BanFooterSettingsSchema, res);
      } catch (e: any) {
        banFooter.error = e.message;
      } finally {
        banFooter.saving = false;
      }
    },

    async loadModmail(guildId: string) {
      modmail.loading = true;
      modmail.error = null;
      try {
        const res = await settingsClient.getModmailSettings({ guildId });
        modmail.data = res;
        modmail.saved = clone(ModmailSettingsSchema, res);
      } catch (e: any) {
        modmail.error = e.message;
      } finally {
        modmail.loading = false;
      }
    },

    async saveModmail() {
      modmail.saving = true;
      modmail.error = null;
      try {
        const res = await settingsClient.updateModmailSettings({ settings: modmail.data });
        modmail.data = res;
        modmail.saved = clone(ModmailSettingsSchema, res);
      } catch (e: any) {
        modmail.error = e.message;
      } finally {
        modmail.saving = false;
      }
    },
  };
}
