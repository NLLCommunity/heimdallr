package main

import (
	"context"
	"fmt"
	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
	"github.com/myrkvi/heimdallr/commands"
	_ "github.com/myrkvi/heimdallr/config"
	"github.com/myrkvi/heimdallr/listeners"
	"github.com/myrkvi/heimdallr/model"
	"github.com/spf13/viper"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	token := viper.GetString("bot.token")
	if token == "" {
		panic("No bot token found in config file. Please set 'bot.token'.")
	}
	_, err := model.InitDB(viper.GetString("bot.db"))
	if err != nil {
		panic(fmt.Errorf("failed to initialize database: %w", err))
	}

	r := handler.New()
	r.Command("/quote", commands.QuoteHandler)
	r.Command("/warn", commands.WarnHandler)
	r.Command("/warnings", commands.UserInfractionsHandler)
	r.Component("/infractions-user/{offset}", commands.UserInfractionButtonHandler)
	r.Command("/ping", commands.PingHandler)
	r.Route("/infractions", func(r handler.Router) {
		r.Command("/list", commands.InfractionsListHandler)
		r.Command("/remove", commands.InfractionsRemoveHandler)
	})
	r.Component("/infractions-mod/{userID}/{offset}", commands.InfractionsListComponentHandler)
	r.Route("/admin", func(r handler.Router) {
		r.Command("/mod-channel/set", commands.AdminModChannelSetCommandHandler)
		r.Command("/mod-channel/clear", commands.AdminModChannelClearCommandHandler)
		r.Command("/mod-channel/get", commands.AdminModChannelGetCommandHandler)
		r.Command("/infraction-half-life/set", commands.AdminInfractionHalfLifeSetCommandHandler)
		r.Command("/infraction-half-life/get", commands.AdminInfractionHalfLifeGetCommandHandler)
		r.Command("/infraction-half-life/clear", commands.AdminInfractionHalfLifeClearCommandHandler)
		r.Command("/notify-on-warned-user-join/set", commands.AdminNotifyOnWarnedUserJoinSetCommandHandler)
		r.Command("/notify-on-warned-user-join/get", commands.AdminNotifyOnWarnedUserJoinGetCommandHandler)

		r.Route("/gatekeep", func(r handler.Router) {
			r.Command("/enabled", commands.AdminGatekeepEnabledCommandHandler)
			r.Command("/info", commands.AdminGatekeepInfoCommandHandler)
			r.Command("/pending-role", commands.AdminGatekeepPendingRoleSetCommandHandler)
			r.Command("/approved-role", commands.AdminGatekeepApprovedRoleSetCommandHandler)
			r.Command("/give-pending-role-on-join", commands.AdminGatekeepAddPendingRoleOnJoinSetCommandHandler)
			r.Command("/approved-message", commands.AdminGatekeepApprovedMessageSetCommandHandler)
		})

		r.Route("/join-leave", func(r handler.Router) {
			r.Command("/info", commands.AdminJoinLeaveInfoCommandHandler)
			r.Command("/join-enabled", commands.AdminJoinLeaveSetJoinEnabledCommandHandler)
			r.Command("/join-message", commands.AdminJoinLeaveSetJoinMessageCommandHandler)
			r.Command("/leave-enabled", commands.AdminJoinLeaveSetLeaveEnabledCommandHandler)
			r.Command("/leave-message", commands.AdminJoinLeaveSetLeaveMessageCommandHandler)
			r.Command("/channel", commands.AdminJoinLeaveSetChannelCommandHandler)

			r.Modal("/join-message/modal", commands.AdminJoinLeaveJoinMessageModal)
			r.Modal("/leave-message/modal", commands.AdminJoinLeaveLeaveMessageModal)
		})
	})

	commandCreates := []discord.ApplicationCommandCreate{
		commands.QuoteCommand,
		commands.WarnCommand,
		commands.UserInfractionsCommand,
		commands.PingCommand,
		commands.InfractionsCommand,
		commands.AdminCommand,
	}

	client, err := disgo.New(token,
		bot.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))),
		bot.WithDefaultGateway(),
		bot.WithEventListeners(r),
		bot.WithEventListenerFunc(func(e *events.Ready) {
			fmt.Println("Bot is ready!")
		}),
		bot.WithEventListenerFunc(listeners.OnWarnedUserJoin),
		bot.WithEventListenerFunc(listeners.TestEvent),
		bot.WithEventListenerFunc(listeners.OnUserJoin),
		bot.WithEventListenerFunc(listeners.OnUserLeave),
		bot.WithGatewayConfigOpts(gateway.WithIntents(gateway.IntentsAll)),
		bot.WithCacheConfigOpts(
			cache.WithCaches(cache.FlagsAll),
		),
	)

	if err != nil {
		panic(fmt.Errorf("failed to create disgo client: %w", err))
	}
	defer client.Close(context.Background())

	var devGuilds []snowflake.ID
	if viper.GetBool("dev_mode.enabled") {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		devGuilds = append(devGuilds, snowflake.ID(viper.GetUint64("dev_mode.guild_id")))
	}

	err = handler.SyncCommands(
		client,
		commandCreates,
		devGuilds,
	)
	if err != nil {
		panic(fmt.Errorf("failed to sync commands: %w", err))
	}

	err = client.OpenGateway(context.Background())
	if err != nil {
		panic(fmt.Errorf("failed to open gateway: %w", err))
	}

	s := make(chan os.Signal, 1)
	signal.Notify(s, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-s
}
