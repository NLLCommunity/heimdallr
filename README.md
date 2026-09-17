# Heimdallr

Heimdallr is a light bot for doing moderation tasks. It has been developed for the Norwegian Language Learning Discord server, but can be used on any server.

It is developed in Go, and uses the Disgo library for Discord.

## Contributing

As a [Go](https://golang.org) project, you'll need to have Go installed on your machine. You can download it from the [official website](https://golang.org/dl/).

> [!TIP]
> You can use a Go version manager like [g](https://github.com/voidint/g) to easily manage multiple versions of Go on your machine. If you are using a dev container (see below), Go will already be installed for you.

1. To contribute to Heimdallr, you'll need to fork the repository and clone it to your machine. You can do this by running the following command:

    ```bash
    git clone https://github.com/NLLCommunity/heimdallr.git
    cd heimdallr
    ```

> [!TIP]
> If you are using an IDE that supports dev containers, such as [Visual Studio Code](https://code.visualstudio.com/docs/devcontainers/containers) or [JetBrains IDEs](https://www.jetbrains.com/help/go/dev-containers-starting-page.html), you may be prompted to open the project in a dev container.
>
> This will plop you right into a development environment with all the dependencies installed and configured for you, without needing to install Go or any other dependencies on your machine. You can then run the bot and make changes to the code as you would normally.

2. Create a new branch to work on your changes:

    ```bash
    git checkout -b my-changes
    ```

3. You will need a Discord bot token, which you can get from the [Discord Developer Portal](https://discord.com/developers/applications), to run the bot. Make sure to enable the Presence, Server Members, and Message Content Intents in the bot settings.

4. Add the bot to the server on which you want to test the bot using the following URL, replacing `YOUR_CLIENT_ID` with the client ID of your bot:

    ```
    https://discord.com/oauth2/authorize?client_id=YOUR_CLIENT_ID&permissions=1496796310534&integration_type=0&scope=bot+applications.commands
    ```

5. Copy [config.template.toml](config.template.toml) to `config.toml` and fill in the required fields, including the bot token.

6. You can then make your changes to the code and run the bot with the following command:

    ```bash
    go run .
    ```

    You can also run the bot using [air](https://github.com/air-verse/air) to automatically reload the bot when you make changes:

    ```bash
    go install github.com/air-verse/air@latest # Install air, if you haven't already
    air
    ```

7. Once you're happy with your changes, commit, push up your changes, and create a pull request on GitHub.

    ```bash
    git add .
    git commit -m "My changes"
    ```

    If you are working with a fork, you will need to push your changes to your fork and create a pull request from there.

    ```bash
    git remote add my-fork https://github.com/YOUR_USERNAME/heimdallr.git
    git push my-fork my-changes
    ```

    Otherwise, you can push your changes directly to the repository.

    ```bash
    git push origin my-changes
    ```

## Deploying to Heroku

Heimdallr ships with a `Procfile`, `Aptfile`, and `app.json` that target the
`heroku-24` stack. The web dyno serves both the Discord gateway connection
and the admin dashboard.

```bash
heroku create your-heimdallr-app
heroku buildpacks:add heroku-community/apt
heroku buildpacks:add heroku/go
heroku config:set \
  HEIMDALLR_BOT_TOKEN=... \
  HEIMDALLR_DASHBOARD_BASE_URL=https://your-heimdallr-app.herokuapp.com \
  "HEIMDALLR_WEB_TRUSTED_PROXIES=0.0.0.0/0 ::/0"
git push heroku main
```

The Apt buildpack installs Litestream from `Aptfile` so the SQLite database
can be replicated to S3-compatible storage. Heroku's filesystem is ephemeral
— without `DB_REPLICA_URL` (and the matching `LITESTREAM_*` credentials),
state is lost on every restart. See `litestream.yml` for the replica config
and `app.json` for the full list of supported env vars.

## License

[GPL-3.0](LICENSE)

## Starboards

Administrators can open **Starboards** from the server dashboard to create
multiple boards. Each board has a name, its own text channel, a Unicode or
server custom emoji, a positive minimum score, and an enabled switch. The
server-wide starboard switch is off by default. Changing a board's emoji or
destination removes its old copies and reevaluates tracked originals.

Votes on the original and a board's copy are combined independently for that
board. Each non-bot user counts once per sign, even if they react to both
messages. The configured emoji adds one; ❌ subtracts one and cannot be chosen
as the positive emoji. A user reacting with both signs contributes zero.
Self-votes count. For example, six ⭐ and four ❌ yield a score of two.

Qualifying messages use Heimdallr's quote presentation, with current scores
and a link to the original. Source edits update every copy; source deletion
removes them. Falling below the threshold removes that board's copy. Its
reactions disappear with it, so a subsequent change to the original's votes
is required before reposting. There is no server-history scan: reaction
activity discovers messages, and tracked entries are periodically repaired.

The `/starboard` commands let moderators remove/restore an entry and
blacklist/unblacklist messages or channels for one board or the whole server.
Board moderation requires **Manage Messages** in its destination; server-wide
blacklists require server-level **Manage Messages**. Administrators always
have access. Removing a copy directly in Discord suppresses it on that board
until restored. Channel blacklists also cover their threads.

Only sources visible to @everyone are eligible. Bot/webhook messages, private
threads, and starboard destination channels are excluded. Age-restricted
content cannot be copied into an unrestricted destination. The bot needs
View Channel, Read Message History, Send Messages, Embed Links, and Add
Reactions in each destination, plus access to source messages and reactions.

Disabling stops automatic publication and score updates while retaining
existing copies. Source edits/deletions and moderation cleanup still run.
Transient errors are retried; ambiguous sends are recovered before another
copy can be created, to avoid duplicate crossposts.
