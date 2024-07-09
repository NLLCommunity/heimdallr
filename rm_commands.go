package main

import (
	"context"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/snowflake/v2"
	"log/slog"
)

func rmCommands(token string, global bool, guildID uint64) {
	client, err := disgo.New(token)
	if err != nil {
		slog.Error("Failed to create client")
		panic(err)
	}

	defer client.Close(context.Background())

	if global {
		rmGlobal(client)
	}
	if guildID != 0 {
		rmGuild(client, guildID)
	}
}

func rmGlobal(client bot.Client) {
	cmds, err := client.Rest().GetGlobalCommands(client.ApplicationID(), false)
	if err != nil {
		slog.Error("Failed to get global commands")
		panic(err)
	}

	for _, cmd := range cmds {
		slog.Info("Deleting global command", "name", cmd.Name)
		err = client.Rest().DeleteGlobalCommand(
			client.ApplicationID(),
			cmd.ID(),
		)
		if err != nil {
			slog.Error("Failed to delete global command",
				"command_id", cmd.ID(),
				"name", cmd.Name)
			panic(err)
		}
	}
}

func rmGuild(client bot.Client, guildID uint64) {
	cmds, err := client.Rest().GetGuildCommands(client.ApplicationID(), snowflake.ID(guildID), false)
	if err != nil {
		slog.Error("Failed to get guild commands")
		panic(err)
	}

	for _, cmd := range cmds {
		slog.Info("Deleting guild command", "name", cmd.Name, "guild_id", guildID)
		err = client.Rest().DeleteGuildCommand(
			client.ApplicationID(),
			snowflake.ID(guildID),
			cmd.ID(),
		)
		if err != nil {
			slog.Error("Failed to delete guild command",
				"guild_id", guildID,
				"command_id", cmd.ID(),
				"name", cmd.Name)
			panic(err)
		}
	}
}
