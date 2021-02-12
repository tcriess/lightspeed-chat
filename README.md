# Project Lightspeed Chat

[Project Lightspeed](https://github.com/GRVYDEV/Project-Lightspeed) is a live streaming system with very low latency. [Project Lightspeed Chat](https://github.com/tcriess/lightspeed-chat) provides a chat server to
this ecosystem. The objective is to have a simple yet fully functional chat system the integrates with Lightspeed.

# Features

- Authentication: currently guests and authenticated users are supported, authentication is only supported via an Open
  ID Connect provider
- Persistence: messages and translations are persisted on the file system using BuntDB, other persistence backends may
  be supported in the future
- Buit-in translation support: dynamic message translations are fully supported by the chat server, a plugin may provide the
  actual translation text
- Plugins: Hashicorps' go-plugin is used to provide a generic plugin interface. Plugins can process incoming messages
  and emit messages and/or translations based on those, or they can emit messages regularly. A translation plugin using
  the Google translate API is part of this repository (separate configuration and setup of a Google Cloud Platform
  project is required)
- Configuration: Hashicorps' HCL is used as the configuration file language

# Roadmap

Maybe the whole system may one day be replaced by a Matrix server and an embeddable Matrix client.

# Prerequisites

## Project Lightspeed

For live video streaming, the parts of Project Lightspeed are required, that is [Lightspeed-ingest](https://github.com/GRVYDEV/Lightspeed-ingest) and [Lightspeed-webrtc](https://github.com/GRVYDEV/Lightspeed-webrt).
As a frontend, grab the branch `feature-external-chat` from this fork of [Lightspeed-react](https://github.com/tcriess/Lightspeed-react/tree/feature-external-chat).

In order to try the chat, actually only the frontend is required.

# Build

## Chat server

*Note*: Building and running has only be tested on Linux platforms using Go v1.15.
So: YMMV.

Run `build-binaries-prod.sh` in the main directory. Alternatively:

```shell
cd cmd/lightspeed-chat
go build .
```

## Plugins

Execute `go build .` in the directory where the source code of the plugin is located, f.e.

```shell
cd plugins/lightspeed-chat-google-translate-plugin
go build .
```

# Install

## Chat server

Copy the binary `cmd/lightspeed-chat/lightspeed-chat` to a convenient location:

```shell
sudo cp cmd/lightspeed-chat/lightspeed-chat /usr/local/bin
```

## Plugins

Copy the binary of the plugin (
f.e. `plugins/lightspeed-chat-google-translate-plugin/lightspeed-chat-google-translate-plugin`) to a convenient
location:

```shell
mkdir -p ~/.config/lightspeed-chat/plugins
cp plugins/lightspeed-chat-google-translate-plugin/lightspeed-chat-google-translate-plugin ~/.config/lightspeed-chat/plugins
```

## Frontend

Lightspeed-chat requires a tweaked version of the React frontend of the Lightspeed project (branch `feature-external-chat` found [here](https://github.com/tcriess/Lightspeed-react/tree/feature-external-chat)).

```shell
git clone https://github.com/tcriess/Lightspeed-react.git
cd Lightspeed-react
git checkout feature-external-chat
npm install
npm run-script build  # production built is in the "build/" directory
```

To test locally:
```shell
npm start
```

# Configure

Lightspeed-chat uses HCL as configuration language.
The configuration of lightspeed-chat and its plugins can either be put into one HCL file or split into multiple HCL files in the same directory.

```shell
./cmd/lightspeed-chat/lightspeed-chat -c CONFIGURATION
```
where `CONFIGURATION` points to either a single configuration file or a directory, in the latter case all `*.hcl` files in this directory are combined.

## Chat server

```hcl
oidc "google" {
  client_id = ""
  provider_url = "https://accounts.google.com"
}

history {
  history_size = 1000
  translation_history_size = 5000
}

persistence {
  buntdb {
    name = "default.buntdb"
  }
}
```

### Authentication

Optionally, lightspeed-chat can use an Open ID Connect provider to authenticate a user.
OIDC providers are configured in `oidc`-blocks, the required attributes are `client_id` and `provider_url`.
Note that currently the claim `email` is required and used as the user id and nick.
This may change in the future.

### History

The immediate chat history is kept in memory, the length of the ring buffer for the chat messages and the translations is provided in the `history`-block, the attributes are called `history_size` and `translation_history_size`, respectively.
Note that one translation for each configured language is generated per chat message.

### Persistence

As a persistence backend, currently only [BuntDB](https://github.com/tidwall/buntdb) is supported.
BuntDB is an in-memory key/value store persisting the contents to (almost) plain-text files.
The block to configure persistence backends is called `persistence`, the sub-block `buntdb` only requires the attribute `name` defining the file name (relative to the working directory or absolute path).

## Plugins

Lightspeed-chat ships with a plugin which provides google-translate translations.
In order to use it in a live system, you need to register a google cloud project, activate access to the google translate API and set up an OAuth2.0 client id for web applications.
Make sure to include the correct redirect URL(s) for your system (for local testing, this can be something like `http://localhost:3000`).

```hcl
plugin "google-translate" {
  config {
    project_id = "YOURPROJECTID"
    languages = [
      "de-DE",
      "es-ES",
      "en-US"]
    cron_spec = "@every 60m"
    cache_size = 10000
  }
}
```

Plugins are configures in a `plugin`-block labelled with the name of the plugin.
That is, the file name of the binary stripped of the leading `lightspeed-chat-` and the trailing `-plugin`.
Each plugin defines its own configuration requirements which have to be in a `config`-block.

The google translate plugin requires the google cloud project ID (string), the languages to translate into (list of strings), a cron specification (string) - the plugin sends "alive" chat messages according to this cron spec -, and a `cache_size`, as all translations are cached in-memory in an LRU-cache.

Note that in order to actually use the google translate API, the API credentials are also required, the environment variable `GOOGLE_APPLICATION_CREDENTIALS` needs to point to the corresponding JSON-file provided by google.

# Run

## Locally

### Frontend

In the `Lightspeed-react` directory (branch `feature-external-chat`)

```shell
npm start
```

The frontend is now served on `localhost:3000`.

### Chat

Without plugins, single configuration file:

```shell
./cmd/lightspeed-chat/lightspeed-chat -c config/config.hcl
```

With the google translate plugin, multiple configuration files in a directory:

```shell
./cmd/lightspeed-chat/lightspeed-chat -p plugins/lightspeed-chat-google-translate-plugin/lightspeed-chat-google-translate-plugin -c config
```

# Deployment

tbd