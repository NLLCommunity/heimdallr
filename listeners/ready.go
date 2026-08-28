package listeners

import (
	"fmt"
	"os"

	"github.com/NLLCommunity/heimdallr/utils"
	"github.com/disgoorg/disgo/events"
	"golang.org/x/term"
)

func OnReady(e *events.Ready) {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		fmt.Println(utils.ReadyText)
	} else {
		fmt.Println("Heimdallr is ready!")
	}
}
