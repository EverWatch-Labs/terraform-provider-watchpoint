resource "watchpoint_oncall_schedule" "primary" {
  name                 = "Platform Primary"
  rotation_type        = "Weekly"
  rotation_length_days = 7
  handoff_hour_utc     = 16

  participants = [
    {
      user_email = "ada@example.com"
      user_name  = "Ada Lovelace"
    },
    {
      user_email = "grace@example.com"
      user_name  = "Grace Hopper"
    },
  ]
}

# Follow-the-sun cover in New York time: the base rotation covers weekday
# business hours, and a weekend layer with its own people sits above it.
resource "watchpoint_oncall_schedule" "follow_the_sun" {
  name             = "Payments"
  time_zone        = "America/New_York"
  rotation_type    = "Weekly"
  handoff_hour_utc = 9

  restrictions = [
    { days = [1, 2, 3, 4, 5], start_minute = 480, end_minute = 1080 },
  ]

  participants = [
    { user_email = "ada@example.com" },
    { user_email = "grace@example.com" },
  ]

  layers = [
    {
      name                  = "Weekend"
      rotation_type         = "Custom"
      rotation_length_hours = 24
      restrictions = [
        { days = [0, 6], start_minute = 0, end_minute = 1440 },
      ]
      participants = [
        { user_email = "linus@example.com" },
        { user_email = "margaret@example.com" },
      ]
    },
  ]
}
