resource "watchpoint_synthetic_monitor" "example" {
  name        = "Checkout flow"
  start_url   = "https://app.example.com"
  script_type = "Playwright"
  script_json = jsonencode([
    { action = "navigate", url = "https://app.example.com" },
    { action = "assert_text", selector = "h1", value = "Welcome" },
  ])
  check_interval_seconds = 900
}
