package interactions

import (
	"log/slog"
	"runtime/debug"

	"github.com/disgoorg/disgo/handler"
)

// RecoverGo runs the next handler in a goroutine, recovering any panic so that a
// single faulty interaction handler cannot crash the whole bot.
//
// It replaces disgo's middleware.Go, which spawns the handler in a bare
// goroutine. A panic there escapes the event manager's recover and terminates
// the process, taking down every guild and skipping graceful shutdown. Returned
// errors are logged the same way middleware.Go logs them.
func RecoverGo(next handler.Handler) handler.Handler {
	return func(event *handler.InteractionEvent) error {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					event.Client().Logger.Error(
						"recovered from panic in interaction handler",
						slog.Any("panic", r),
						slog.String("stack", string(debug.Stack())),
					)
				}
			}()
			if err := next(event); err != nil {
				event.Client().Logger.Error("failed to handle interaction", slog.Any("err", err))
			}
		}()
		return nil
	}
}
