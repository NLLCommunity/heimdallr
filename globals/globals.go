package globals

import "github.com/disgoorg/snowflake/v2"

var ExcludedFromModKickLog = make(map[snowflake.ID]struct{})
