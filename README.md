# Project Lightspeed Chat

Project Lightspeed is a live streaming system with very low latency. Project Lightspeed Chat provides a chat server to
this ecosystem. The objective is to have a simple yet fully functional chat system the integrates with Lightspeed.

# Features

- Authentication: currently guests and authenticated users are supported, authentication is only supported via an Open
  ID Connect provider
- Persistence: messages and translations are persisted on the file system using BuntDB, other persistence backends may
  be supported in the future
- Buit-in translation: dynamic message translations are fully supported by the chat server, a plugin may provide the
  actual translation text
- Plugins: Hashicorps' go-plugin is used to provide a generic plugin interface. Plugins can process incoming messages
  and emit messages and/or translations based on those, or they can emit messages regularly. A translation plugin using
  the Google translate API is part of this repository (separate configuration and setup of a Google Cloud Platform
  project is required)
- Configuration: Hashicorps' HCL is used as the configuration file language

# Roadmap

Maybe the whole system may one day be replaced by a Matrix server and an embeddable Matrix client.

# Build

## Chat server

Run `build-binaries-prod.sh` in the main directory. Alternatively:

```bash
cd cmd/lightspeed-chat
go build .
```

## Plugins

Execute `go build .` in the directory where the source code of the plugin is located, f.e.

```bash
cd plugins/lightspeed-chat-google-translate-plugin
go build .
```

# Install

## Chat server

Copy the binary `cmd/lightspeed-chat/lightspeed-chat` to a convenient location:

```bash
sudo cp cmd/lightspeed-chat/lightspeed-chat /usr/local/bin
```

## Plugins

Copy the binary of the plugin (
f.e. `plugins/lightspeed-chat-google-translate-plugin/lightspeed-chat-google-translate-plugin`) to a convenient
location:

```bash
mkdir -p ~/.config/lightspeed-chat/plugins
cp plugins/lightspeed-chat-google-translate-plugin/lightspeed-chat-google-translate-plugin ~/.config/lightspeed-chat/plugins
```

# Configure

## Chat server

## Plugins

# Run

# Deployment
