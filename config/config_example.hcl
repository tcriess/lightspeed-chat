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
