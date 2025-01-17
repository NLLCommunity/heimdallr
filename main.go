package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/disgo/handler/middleware"
	"github.com/disgoorg/snowflake/v2"
	"github.com/spf13/viper"

	"github.com/myrkvi/heimdallr/commands"
	"github.com/myrkvi/heimdallr/components"
	_ "github.com/myrkvi/heimdallr/config"
	"github.com/myrkvi/heimdallr/listeners"
	"github.com/myrkvi/heimdallr/model"
	"github.com/myrkvi/heimdallr/scheduled_tasks"
)

var rmGlobalCommands = flag.Bool("rm-global-commands", false, "Remove global commands")
var rmGuildCommands = flag.Uint64("rm-guild-commands", 0, "Remove guild commands for guild specified by ID")

var intents = gateway.IntentGuilds |
	gateway.IntentGuildMembers |
	gateway.IntentGuildModeration |
	gateway.IntentGuildExpressions |
	gateway.IntentGuildIntegrations |
	gateway.IntentGuildWebhooks |
	gateway.IntentGuildInvites |
	gateway.IntentGuildVoiceStates |
	gateway.IntentGuildMessages |
	gateway.IntentGuildMessageReactions |
	gateway.IntentDirectMessages |
	gateway.IntentDirectMessageReactions |
	gateway.IntentMessageContent |
	gateway.IntentGuildScheduledEvents |
	gateway.IntentAutoModerationConfiguration |
	gateway.IntentAutoModerationExecution |
	gateway.IntentGuildMessagePolls |
	gateway.IntentDirectMessagePolls

func main() {
	flag.Parse()
	token := viper.GetString("bot.token")
	if token == "" {
		panic("No bot token found in config file. Please set 'bot.token'.")
	}

	if *rmGlobalCommands || *rmGuildCommands != 0 {
		slog.Info("Removing commands.")
		rmCommands(token, *rmGlobalCommands, *rmGuildCommands)
		return
	}

	_, err := model.InitDB(viper.GetString("bot.db"))
	if err != nil {
		panic(fmt.Errorf("failed to initialize database: %w", err))
	}

	r := handler.New()

	r.Use(middleware.Go)

	r.Command("/ping", commands.PingHandler)
	r.Command("/quote", commands.QuoteHandler)

	r.Command("/warn", commands.WarnHandler)
	r.Command("/warnings", commands.UserInfractionsHandler)
	r.Route("/infractions", func(r handler.Router) {
		r.Command("/list", commands.InfractionsListHandler)
		r.Command("/remove", commands.InfractionsRemoveHandler)
	})
	r.Component("/infractions-user/{offset}", commands.UserInfractionButtonHandler)

	r.Component("/infractions-mod/{userID}/{offset}", commands.InfractionsListComponentHandler)
	r.Route("/admin", func(r handler.Router) {
		r.Component("/show-all-button", commands.AdminShowAllButtonHandler)
		r.Command("/info", commands.AdminInfoHandler)
		r.Command("/mod-channel", commands.AdminModChannelHandler)
		r.Command("/infractions", commands.AdminInfractionsHandler)
		r.Command("/gatekeep", commands.AdminGatekeepHandler)
		r.Command("/gatekeep-message", commands.AdminGatekeepMessageHandler)
		r.Component("/gatekeep-message/button", commands.AdminGatekeepMessageButtonHandler)
		r.Modal("/gatekeep-message/modal", commands.AdminGatekeepMessageModalHandler)
		r.Command("/join-leave", commands.AdminJoinLeaveHandler)

		r.Command("/join-message", commands.AdminJoinMessageHandler)
		r.Component("/join-message/button", commands.AdminJoinMessageButtonHandler)
		r.Modal("/join-message/modal", commands.AdminJoinMessageModalHandler)

		r.Command("/leave-message", commands.AdminLeaveMessageHandler)
		r.Component("/leave-message/button", commands.AdminLeaveMessageButtonHandler)
		r.Modal("/leave-message/modal", commands.AdminLeaveMessageModalHandler)

		r.Command("/anti-spam", commands.AdminAntiSpamHandler)
	})

	r.Command("/Approve", commands.ApproveUserCommandHandler)
	r.Command("/approve", commands.ApproveSlashCommandHandler)

	r.Command("/kick/with-message", commands.KickWithMessageHandler)
	r.Command("/ban/with-message", commands.BanWithMessageHandler)
	r.Command("/ban/until", commands.BanUntilHandler)

	r.Command("/create-role-button", commands.CreateRoleButtonHandler)
	r.Component("/role/assign/{roleID}", components.RoleAssignButtonHandler)

	r.Command("/prune-pending-members", commands.PruneHandler)
	r.Command("/prune-pending-members-dry-run", commands.PruneDryRunHandler)

	commandCreates := []discord.ApplicationCommandCreate{
		commands.PingCommand,
		commands.QuoteCommand,
		commands.WarnCommand,
		commands.UserInfractionsCommand,
		commands.InfractionsCommand,
		commands.AdminCommand,
		commands.KickCommand,
		commands.BanCommand,
		commands.ApproveSlashCommand,
		commands.ApproveUserCommand,
		commands.CreateRoleButtonCommand,
		commands.PruneCommand,
		commands.PruneDryRunCommand,
	}

	client, err := disgo.New(token,
		bot.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: getLogLevel(viper.GetString("loglevel")),
		}))),
		bot.WithDefaultGateway(),
		bot.WithEventListeners(r),
		bot.WithEventListenerFunc(func(e *events.Ready) {
			fmt.Println("Bot is ready!")
		}),
		bot.WithEventListenerFunc(listeners.OnWarnedUserJoin),
		bot.WithEventListenerFunc(listeners.OnGatekeepUserJoin),
		bot.WithEventListenerFunc(listeners.OnUserJoin),
		bot.WithEventListenerFunc(listeners.OnUserLeave),
		bot.WithEventListenerFunc(listeners.OnMemberBan),
		bot.WithEventListenerFunc(listeners.OnAuditLog),
		bot.WithEventListenerFunc(listeners.OnAntispamMessageCreate),
		bot.WithGatewayConfigOpts(gateway.WithIntents(intents)),
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

	removeTempBansTask := scheduled_tasks.RemoveTempBansScheduledTask(client)

	s := make(chan os.Signal, 1)
	signal.Notify(s, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-s
	removeTempBansTask.Stop()
}

func getLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
