import { createClient } from "@connectrpc/connect";
import { transport } from "./transport";
import { AuthService } from "../../gen/heimdallr/v1/auth_pb";
import { GuildSettingsService } from "../../gen/heimdallr/v1/guild_settings_pb";

export const authClient = createClient(AuthService, transport);
export const settingsClient = createClient(GuildSettingsService, transport);
