# Infraction system explained
Heimdallr's infraction system is a way for moderators to communicate to users 
and themselves that a user has behaved in an unacceptable manner. They can be
issued with some variety, depending on various factors.

An infraction can be set as silent, which means that Heimdallr will not attempt
to send a message to the user, informing them of the infraction. It is still
logged, and the user can still see it using the `/warnings` command. This is
useful when the user has been informed in various other ways, such as in a
conversation with a moderator or in one of the server's channels.

An infraction can have a severity (or weighting, or "strikes") assigned to it.
By default, the severity is 1.0. If one were to have a sort of "three strikes"
system, then three infractions with the default severity of 1.0 would result in
some action being taken. However, some unacceptable behaviour may be more or less
severe than others, and as such, one can set the severity.

**NLL Moderator's Note:** While a "three strikes" system is a good guideline, it
should not necessarily be a hard rule. This is especially true for systems that
do not provide this manner of granularity, but we advice that moderators should
act on the data that they have, and we hope that this helps to do so.

If configured to do so, an infraction's severity may also decrease over time.
The reasoning for this is that if a user is warned, and takes the warning
seriously, they are unlikely to repeat the behaviour. On the flip side, a
problematic user is likely to not take the warning seriously, and is likely to
repeat their behaviour, often within a shorter time frame. As such, an infraction
that was issued some time ago should not count for something that just happened.

This is configured by setting the half-life in the infraction system. If the
half-life is set to 90 days, then a warning with the default severity of 1.0
will go down to 0.5 after 90 days.
