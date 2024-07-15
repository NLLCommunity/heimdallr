# Introduction
Heimdallr is a light moderation bot, primarily written for the Norwegian Language
Learning Community. Together with the [Modmail bot](https://nll-modmail.fly.dev/),
they form the moderation system for the NLL Discord server.

For moderation and server administration, Heimdallr provides: an infraction system
with some unique features, a gatekeep system used for manual approval of users, and
a join/leave message system.

Modmail adds a simple thread-based mail system in addition, without storing *any*
messages or data about the server or its users.


Both Heimdallr and Modmail are written in Go, and utilise the [Disgo](https://github.com/disgoorg/disgo)
library for Discord interactions.
