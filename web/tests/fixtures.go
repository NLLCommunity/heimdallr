//go:build ignore

// Render real templates for offline browser integration tests.
package main

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/NLLCommunity/heimdallr/model"
	"github.com/NLLCommunity/heimdallr/web/templates/components"
	"github.com/NLLCommunity/heimdallr/web/templates/layouts"
	"github.com/NLLCommunity/heimdallr/web/templates/pages"
	"github.com/NLLCommunity/heimdallr/web/templates/partials"
	"github.com/a-h/templ"
)

func main() {
	ctx := context.Background()
	write := func(name string, component templ.Component) {
		f, err := os.Create(filepath.Join(os.Args[1], name+".html"))
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if err := component.Render(ctx, f); err != nil {
			panic(err)
		}
	}
	channels := []components.ChannelGroup{{Channels: []components.ChannelInfo{{ID: "123", Name: "announcements"}}}}
	write("post", pages.PostEditor(layouts.NavData{ExtraScripts: []string{"post-editor.js"}}, pages.PostEditorData{
		GuildID: "1", Channels: channels,
		Post: model.Post{ID: 1, Version: 2, Name: "Test post", ChannelID: 123,
			ComponentsJSON: `{"version":1,"messages":[{"components":[{"type":10,"content":"First message","id":15}]},{"components":[{"type":10,"content":"Second message"}]}]}`},
	}))
	write("sandbox", pages.Sandbox(layouts.NavData{}, pages.SandboxData{GuildID: "1", Channels: channels}))
	settings := partials.SettingsGatekeep(partials.GatekeepData{GuildID: "1", ApprovedMessageV2: true, ApprovedMessageV2Json: `[{"type":10,"content":"Hello {{user}}","id":17}]`})
	birthday := partials.SettingsBirthday(partials.BirthdayData{GuildID: "1", MessageV2: false, MessageV2Json: "[]", Channels: channels})
	write("settings-fragment", settings)
	write("birthday-fragment", birthday)
	// Supply the settings fragments as children through the templ context.
	f, err := os.Create(filepath.Join(os.Args[1], "settings.html"))
	if err != nil {
		panic(err)
	}
	defer f.Close()
	child := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if err := settings.Render(ctx, w); err != nil {
			return err
		}
		return birthday.Render(ctx, w)
	})
	if err := layouts.Base("Settings", layouts.NavData{}).Render(templ.WithChildren(ctx, child), f); err != nil {
		panic(err)
	}
}
