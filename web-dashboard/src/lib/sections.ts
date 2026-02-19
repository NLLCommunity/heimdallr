import type { Component } from "svelte";
import ModChannelSection from "./components/settings/ModChannelSection.svelte";
import InfractionsSection from "./components/settings/InfractionsSection.svelte";
import GatekeepSection from "./components/settings/GatekeepSection.svelte";
import JoinLeaveSection from "./components/settings/JoinLeaveSection.svelte";
import AntiSpamSection from "./components/settings/AntiSpamSection.svelte";
import BanFooterSection from "./components/settings/BanFooterSection.svelte";
import ModmailSection from "./components/settings/ModmailSection.svelte";
import PaceControlSection from "./components/settings/PaceControlSection.svelte";

export interface SectionDef {
  id: string;
  label: string;
  component: Component;
}

export const sections: SectionDef[] = [
  { id: "mod-channel", label: "Moderator Channel", component: ModChannelSection },
  { id: "infractions", label: "Infractions", component: InfractionsSection },
  { id: "gatekeep", label: "Gatekeep", component: GatekeepSection },
  { id: "join-leave", label: "Join/Leave Messages", component: JoinLeaveSection },
  { id: "anti-spam", label: "Anti-Spam", component: AntiSpamSection },
  { id: "ban-footer", label: "Ban Footer", component: BanFooterSection },
  { id: "modmail", label: "Modmail", component: ModmailSection },
  { id: "pace-control", label: "Pace Control", component: PaceControlSection },
];
