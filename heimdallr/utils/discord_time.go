package utils

import (
	"fmt"
	"time"
)

type DiscordTime struct {
	time.Time
}

func (t DiscordTime) ToRelative() string {
	return fmt.Sprintf("<t:%d:R>", t.UTC().Unix())
}

func (t DiscordTime) ToShortTime() string {
	return fmt.Sprintf("<t:%d:t>", t.UTC().Unix())
}

func (t DiscordTime) ToLongTime() string {
	return fmt.Sprintf("<t:%d:T>", t.UTC().Unix())
}

func (t DiscordTime) ToShortDate() string {
	return fmt.Sprintf("<t:%d:d>", t.UTC().Unix())
}

func (t DiscordTime) ToLongDate() string {
	return fmt.Sprintf("<t:%d:D>", t.UTC().Unix())
}

func (t DiscordTime) ToShortDateTime() string {
	return fmt.Sprintf("<t:%d:f>", t.UTC().Unix())
}

func (t DiscordTime) ToLongDateTime() string {
	return fmt.Sprintf("<t:%d:F>", t.UTC().Unix())
}
