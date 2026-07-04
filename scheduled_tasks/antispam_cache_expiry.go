package scheduled_tasks

import (
	"context"
	"time"

	"github.com/NLLCommunity/heimdallr/listeners"
	"github.com/NLLCommunity/heimdallr/task"
)

func RemoveExpiredMessagesInTTLCache() task.Task {
	t := task.New("remove-expired-messages-antispam", removeExpiredMessages, nil, 1*time.Minute, true)
	t.StartNoWait()

	return t
}

func removeExpiredMessages(ctx context.Context) {
	listeners.RemoveExpiredMessagesInTTLCache()
}
