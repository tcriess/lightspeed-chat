oidc "google" {
  client_id = ""
  provider_url = "https://accounts.google.com"
}

history {
  history_size = 1000
}

persistence {
  buntdb {
    name = "default.buntdb"
  }
}
