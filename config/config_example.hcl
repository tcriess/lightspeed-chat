oidc "google" {
  client_id = ""
  provider_url = "https://accounts.google.com"
}

history {
  history_size = 1000
}

persistence {
  buntdb {
    global_name = "default.buntdb"
    room_name_template = "room_{{ .RoomId }}.buntdb"
  }
}
