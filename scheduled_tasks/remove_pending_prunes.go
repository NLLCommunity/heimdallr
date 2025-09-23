package scheduled_tasks

import (
	"context"
	"time"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/task"
)

func RemoveStalePendingPrunes() task.Task {
	t := task.New("remove-stale-prunes", removeStalePrunes, nil, 1*time.Hour)
	t.StartNoWait()

	return t
}

func removeStalePrunes(ctx context.Context) {
	cutoff := time.Now().Add(-4 * time.Hour)
	_ = model.DeletePrunesBeforeTime(cutoff)

}
