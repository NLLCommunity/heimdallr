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

	"github.com/NLLCommunity/heimdallr/audit"
	_ "github.com/NLLCommunity/heimdallr/config"
	"github.com/NLLCommunity/heimdallr/interactions"
	"github.com/NLLCommunity/heimdallr/interactions/admin"
	"github.com/NLLCommunity/heimdallr/interactions/admin_dashboard"
	"github.com/NLLCommunity/heimdallr/interactions/ban"
	"github.com/NLLCommunity/heimdallr/interactions/gatekeep"
	"github.com/NLLCommunity/heimdallr/interactions/infractions"
	"github.com/NLLCommunity/heimdallr/interactions/kick"
	"github.com/NLLCommunity/heimdallr/interactions/modmail"
	"github.com/NLLCommunity/heimdallr/interactions/ping"
	"github.com/NLLCommunity/heimdallr/interactions/post_dashboard"
	"github.com/NLLCommunity/heimdallr/interactions/prune"
	"github.com/NLLCommunity/heimdallr/interactions/quote"
	"github.com/NLLCommunity/heimdallr/interactions/role_button"
	"github.com/NLLCommunity/heimdallr/listeners"
	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/scheduled_tasks"
	"github.com/NLLCommunity/heimdallr/web"
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

	_, err := model.InitDB(viper.GetString("bot.db") + "?_journal_mode=WAL&_pragma=analysis_limit(400)")
	if err != nil {
		panic(fmt.Errorf("failed to initialize database: %w", err))
	}

	r := handler.New()
	r.Use(middleware.Go)

	commandInteractions := []interactions.ApplicationCommandRegisterFunc{
		admin.Register,
		admin_dashboard.Register,
		post_dashboard.Register,
		ban.Register,
		gatekeep.Register,
		infractions.Register,
		kick.Register,
		ping.Register,
		prune.Register,
		quote.Register,
		role_button.Register,
		modmail.Register,
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
		bot.WithEventListenerFunc(listeners.OnAuditLogKick),
		bot.WithEventListenerFunc(listeners.OnAntispamMessageCreate),
		bot.WithEventListenerFunc(listeners.OnAuditMemberUpdate),
		bot.WithEventListenerFunc(listeners.OnAuditMessageUpdate),
		bot.WithEventListenerFunc(listeners.OnAuditMessageDelete),
		bot.WithEventListenerFunc(listeners.OnAuditMemberBan),
		bot.WithEventListenerFunc(listeners.OnAuditGuildUnban),
		bot.WithEventListenerFunc(listeners.OnAuditNativeEnrichment),
		bot.WithGatewayConfigOpts(gateway.WithIntents(intents)),
		bot.WithCacheConfigOpts(
			cache.WithCaches(cache.FlagsAll),
		),
	)

	if err != nil {
		panic(fmt.Errorf("failed to create disgo client: %w", err))
	}

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

	// Record the /post-dashboard command ID so the web dashboard can fetch
	// per-guild permission overrides immediately, instead of waiting for a
	// non-admin user to discover the dashboard via the slash command first.
	var registered []discord.ApplicationCommand
	if len(devGuilds) > 0 {
		registered, err = client.Rest.GetGuildCommands(client.ApplicationID, devGuilds[0], false)
	} else {
		registered, err = client.Rest.GetGlobalCommands(client.ApplicationID, false)
	}
	if err != nil {
		slog.Warn("failed to fetch registered commands; /post-dashboard ID will be captured on first use", "error", err)
	} else {
		post_dashboard.SetCommandID(registered)
	}

	err = client.OpenGateway(context.Background())
	if err != nil {
		panic(fmt.Errorf("failed to open gateway: %w", err))
	}

	removeTempBansTask := scheduled_tasks.RemoveTempBansScheduledTask(client)
	removeStalePrunesTask := scheduled_tasks.RemoveStalePendingPrunes()
	pruneAuditLogTask := scheduled_tasks.PruneAuditLogScheduledTask()

	webCtx, cancelWeb := context.WithCancel(context.Background())
	defer cancelWeb()
	webDone := make(chan struct{})
	go func() {
		defer close(webDone)
		if err := web.StartServer(webCtx, viper.GetString("web.address"), client); err != nil {
			slog.Error("Web server stopped with error", "error", err)
		}
	}()

	s := make(chan os.Signal, 1)
	signal.Notify(s, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-s
	slog.Info("Shutdown signal received")
	removeTempBansTask.Stop()
	removeStalePrunesTask.Stop()
	pruneAuditLogTask.Stop()
	// Close ONLY the gateway first so listeners stop firing and can't
	// refill the audit buffer after the flush below. We deliberately keep
	// the REST client and caches alive: in-flight web requests still need
	// them to render audit log pages, channel names, etc. — closing the
	// whole client here would surface as REST errors mid-response.
	client.Gateway.Close(context.Background())
	// Commit any audit log entries still in the pending-enrichment buffer
	// before the process exits. Best-effort: failures inside FlushPending
	// are logged at warn but don't block shutdown.
	audit.FlushPending()
	cancelWeb()
	<-webDone
	// All writers stopped; refresh the SQLite query planner stats per the
	// upstream recommendation for connection close. No-op when nothing
	// has changed since the last run, so cheap to call unconditionally.
	if err := model.DB.Exec("PRAGMA optimize").Error; err != nil {
		slog.Warn("PRAGMA optimize at shutdown failed", "err", err)
	}
	// Now that the web server has drained, the REST client can go.
	// client.Close also calls Gateway.Close, which is idempotent.
	client.Close(context.Background())
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
