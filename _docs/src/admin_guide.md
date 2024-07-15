# Server Administrator Guide
Most settings can be viewed by using the `/admin info` command. Executing this
command will display most settings in an ephemeral message (visible to only the
user who executed the command).

## Moderator Channel
The moderator channel is the channel in which Heimdallr will send messages,
notifications, and other information intended for moderators and administrators
of a server. This should be a channel that is not visible to normal users.

The moderator channel can be viewed and set by using the `/admin mod-channel` command.
Without the optional `channel` argument, the current moderator channel will be displayed.
With it, the moderator channel will be set to the specified channel.

## Infractions
Infractions are warnings that can be viewed by the user who was issued the warning,
in addition to moderators and administrators. Infractions can be viewed by using the 
`/infractions list` command. Infractions can be removed by using the
`/infractions remove` command, and the ID of the infraction. Finally, they can be
issued to a user by using the `/warn` command.

### Infraction Severity and Half-Life
Heimdallr has a severity system for the warnings it issues, which can serve as
a strike system, or something similar. It can also be referred to as *infraction weight*
or strikes. By default, an infraction has a severity of 1.0, but can be overridden
when issuing a warning.

Heimdallr has a system that, over time, decreases the "severity" of infractions.
This intends to make past infractions count for less, as the user has likely 
learned from it, if no more infractions are issued or other actions are taken.

The severity will be halved every X days, where X is the half-life time. If the 
half-life time is set to 30 days, then the severity will be halved every 30 days.
If it set to zero, then the severity will never be halved.

You can also choose to notify the moderator channel when a previous member of
the server has rejoined, and has a total severity of at least a configurable threshold.
Setting the threshold to 0 will notify on all infractions.

## Gatekeep
Gatekeep is a system that enforces manual approval of users before they are able
to join the server. This is useful for moderators who want to ensure that users
meet certain criteria before they are able to join the server.

Gatekeep settings can be viewed by using the `/admin gatekeep` command with no
arguments. Adding optional arguments will change the current settings.

- Gatekeep enabled: Whether to enable the gatekeep system.
- Gatekeep pending role: The role to give to users pending approval, if give 
  pending role is enabled. It will still be removed from users who have it when
  they are approved.
- Gatekeep approved role: The role to give to approved users.
- Gatekeep add pending role on join: Whether to give the pending role to users when they join.

The gatekeep approved message, sent when a user is approved, can be viewed and
set by using the `/admin gatekeep-message` command.


## Join/Leave Messages
Heimdallr can send messages whenever a user joins or leaves the server. These
messages can be viewed and set by using the `/admin join-message` and
`/admin leave-message` commands. The messages can be enabled or disabled, and
the channel to send the messages to can be set using the `/admnin join-leave` command.