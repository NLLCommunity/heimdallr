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

	_ "github.com/NLLCommunity/heimdallr/config"
	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/interactions/admin"
	"github.com/NLLCommunity/heimdallr/interactions/admin_dashboard"
	"github.com/NLLCommunity/heimdallr/interactions/ban"
	"github.com/NLLCommunity/heimdallr/interactions/gatekeep"
	"github.com/NLLCommunity/heimdallr/interactions/infractions"
	"github.com/NLLCommunity/heimdallr/interactions/kick"
	"github.com/NLLCommunity/heimdallr/interactions/modmail"
	"github.com/NLLCommunity/heimdallr/interactions/pace_control"
	"github.com/NLLCommunity/heimdallr/interactions/ping"
	"github.com/NLLCommunity/heimdallr/interactions/prune"
	"github.com/NLLCommunity/heimdallr/interactions/quote"
	"github.com/NLLCommunity/heimdallr/interactions/role_button"
	"github.com/NLLCommunity/heimdallr/listeners"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/rpcserver"
	"github.com/NLLCommunity/heimdallr/scheduled_tasks"
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

	commandInteractions := []interactions.ApplicationCommandRegisterFunc{
		admin.Register,
		admin_dashboard.Register,
		ban.Register,
		gatekeep.Register,
		infractions.Register,
		kick.Register,
		ping.Register,
		prune.Register,
		quote.Register,
		role_button.Register,
		modmail.Register,
		pace_control.Register,
	}

	var commandCreates []discord.ApplicationCommandCreate

	for _, register := range commandInteractions {
		commandCreates = append(commandCreates, register(r)...)
	}

	client, err := disgo.New(
		token,
		bot.WithLogger(
			slog.New(
				slog.NewTextHandler(
					os.Stderr, &slog.HandlerOptions{
						Level: getLogLevel(viper.GetString("loglevel")),
					},
				),
			),
		),
		bot.WithDefaultGateway(),
		bot.WithEventListeners(r),
		bot.WithEventListenerFunc(
			func(e *events.Ready) {
				fmt.Println("Bot is ready!")
			},
		),
		bot.WithEventListenerFunc(listeners.OnWarnedUserJoin),
		bot.WithEventListenerFunc(listeners.OnGatekeepUserJoin),
		bot.WithEventListenerFunc(listeners.OnUserJoin),
		bot.WithEventListenerFunc(listeners.OnUserLeave),
		bot.WithEventListenerFunc(listeners.OnMemberBan),
		bot.WithEventListenerFunc(listeners.OnAuditLog),
		bot.WithEventListenerFunc(listeners.OnAntispamMessageCreate),
		bot.WithEventListenerFunc(listeners.OnPaceControlMessageCreate),
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
	removeStalePrunesTask := scheduled_tasks.RemoveStalePendingPrunes()
	paceControlTask := scheduled_tasks.PaceControlTask(client)

	go func() {
		if err := rpcserver.StartServer(viper.GetString("rpc.address"), client); err != nil {
			slog.Error("Failed to start RPC server", "error", err)
		}
	}()

	s := make(chan os.Signal, 1)
	signal.Notify(s, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-s
	removeTempBansTask.Stop()
	removeStalePrunesTask.Stop()
	paceControlTask.Stop()
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
