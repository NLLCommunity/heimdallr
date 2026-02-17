import { create, clone, type Message } from "@bufbuild/protobuf";
import type { GenMessage } from "@bufbuild/protobuf/codegenv2";
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

export interface SectionState<T> {
  data: T;
  saved: T;
  saving: boolean;
  loading: boolean;
  error: string | null;
}

function makeDefault<T extends Message>(schema: GenMessage<T>): SectionState<T> {
  return {
    data: create(schema),
    saved: create(schema),
    saving: false,
    loading: false,
    error: null,
  };
}

async function loadSection<T extends Message>(
  section: SectionState<T>,
  schema: GenMessage<T>,
  request: () => Promise<T>,
) {
  section.loading = true;
  section.error = null;
  try {
    const res = await request();
    section.data = res;
    section.saved = clone(schema, res);
  } catch (e: any) {
    section.error = e.message;
  } finally {
    section.loading = false;
  }
}

async function saveSection<T extends Message>(
  section: SectionState<T>,
  schema: GenMessage<T>,
  request: () => Promise<T>,
) {
  section.saving = true;
  section.error = null;
  try {
    const res = await request();
    section.data = res;
    section.saved = clone(schema, res);
  } catch (e: any) {
    section.error = e.message;
  } finally {
    section.saving = false;
  }
}

let modChannel = $state<SectionState<ModChannelSettings>>(makeDefault(ModChannelSettingsSchema));
let infractions = $state<SectionState<InfractionSettings>>(makeDefault(InfractionSettingsSchema));
let gatekeep = $state<SectionState<GatekeepSettings>>(makeDefault(GatekeepSettingsSchema));
let joinLeave = $state<SectionState<JoinLeaveSettings>>(makeDefault(JoinLeaveSettingsSchema));
let antiSpam = $state<SectionState<AntiSpamSettings>>(makeDefault(AntiSpamSettingsSchema));
let banFooter = $state<SectionState<BanFooterSettings>>(makeDefault(BanFooterSettingsSchema));
let modmail = $state<SectionState<ModmailSettings>>(makeDefault(ModmailSettingsSchema));

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
      await loadSection(modChannel, ModChannelSettingsSchema,
        () => settingsClient.getModChannel({ guildId }));
    },
    async saveModChannel() {
      await saveSection(modChannel, ModChannelSettingsSchema,
        () => settingsClient.updateModChannel({ settings: modChannel.data }));
    },

    async loadInfractions(guildId: string) {
      await loadSection(infractions, InfractionSettingsSchema,
        () => settingsClient.getInfractionSettings({ guildId }));
    },
    async saveInfractions() {
      await saveSection(infractions, InfractionSettingsSchema,
        () => settingsClient.updateInfractionSettings({ settings: infractions.data }));
    },

    async loadGatekeep(guildId: string) {
      await loadSection(gatekeep, GatekeepSettingsSchema,
        () => settingsClient.getGatekeepSettings({ guildId }));
    },
    async saveGatekeep() {
      await saveSection(gatekeep, GatekeepSettingsSchema,
        () => settingsClient.updateGatekeepSettings({ settings: gatekeep.data }));
    },

    async loadJoinLeave(guildId: string) {
      await loadSection(joinLeave, JoinLeaveSettingsSchema,
        () => settingsClient.getJoinLeaveSettings({ guildId }));
    },
    async saveJoinLeave() {
      await saveSection(joinLeave, JoinLeaveSettingsSchema,
        () => settingsClient.updateJoinLeaveSettings({ settings: joinLeave.data }));
    },

    async loadAntiSpam(guildId: string) {
      await loadSection(antiSpam, AntiSpamSettingsSchema,
        () => settingsClient.getAntiSpamSettings({ guildId }));
    },
    async saveAntiSpam() {
      await saveSection(antiSpam, AntiSpamSettingsSchema,
        () => settingsClient.updateAntiSpamSettings({ settings: antiSpam.data }));
    },

    async loadBanFooter(guildId: string) {
      await loadSection(banFooter, BanFooterSettingsSchema,
        () => settingsClient.getBanFooterSettings({ guildId }));
    },
    async saveBanFooter() {
      await saveSection(banFooter, BanFooterSettingsSchema,
        () => settingsClient.updateBanFooterSettings({ settings: banFooter.data }));
    },

    async loadModmail(guildId: string) {
      await loadSection(modmail, ModmailSettingsSchema,
        () => settingsClient.getModmailSettings({ guildId }));
    },
    async saveModmail() {
      await saveSection(modmail, ModmailSettingsSchema,
        () => settingsClient.updateModmailSettings({ settings: modmail.data }));
    },
  };
}
