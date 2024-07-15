# Moderator Guide

## Banning and kicking
For kicking and banning users, we encourage you to use Discord's native features,
which include the `/ban` and `/kick` commands. However, Heimdallr provides the
following commands which add some extra functionality:

- `/kick with-message`: Kick a user, sending a message to the user immediately 
  before the kick.
- `/ban with-message`: Ban a user, sending a message to the user immediately
  before the ban.
- `/ban until`: Ban a user from the server for a specified amount of time. Can 
  optionally also send a ban reason to the user. Will attempt to DM the user
  about the expiry of the ban.

## Infractions

- `/warn`: Warn a user.
- `/infractions list`: View a user's warnings.
- `/infractions remove`: Remove a user's warning.

## Gatekeep

- `/approve`: Approve a user to join the server. This is also available as a
  user command, so that you can right click or long press a user and select
  "Approve" from the context menu (under "Apps").