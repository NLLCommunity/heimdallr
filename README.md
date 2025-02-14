# Heimdallr

Heimdallr is a light bot for doing moderation tasks. It has been developed for the Norwegian Language Learning Discord server, but can be used on any server.

It is developed in Go, and uses the Disgo library for Discord.

## Contributing

As a [Go](https://golang.org) project, you'll need to have Go installed on your machine. You can download it from the [official website](https://golang.org/dl/).

> [!TIP]
> You can use a Go version manager like [g](https://github.com/voidint/g) to easily manage multiple versions of Go on your machine.

1. To contribute to Heimdallr, you'll need to fork the repository and clone it to your machine. You can do this by running the following command:

    ```bash
    git clone https://github.com/NLLCommunity/heimdallr.git
    cd heimdallr
    ```

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

## License

[GPL-3.0](LICENSE)
