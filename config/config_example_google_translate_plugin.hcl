plugin "google-translate" {
  config {
    project_id = ""
    languages = [
      "de-DE",
      "es-ES",
      "en-US"]
    cron_spec = "@every 60m"
    cache_size = 10000
  }
}
