//go:build !web

package rpcserver

import (
	"log/slog"

	"github.com/disgoorg/disgo/bot"
)

func StartServer(addr string, client *bot.Client) error {
	slog.Info("RPC Server and Web not available")
	return nil
}
