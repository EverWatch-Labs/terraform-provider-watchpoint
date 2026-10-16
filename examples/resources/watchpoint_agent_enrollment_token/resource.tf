# A single-use token that enrolls one host as an infrastructure agent.
resource "watchpoint_agent_enrollment_token" "host" {
  roles = ["infrastructure-agent"]
}

# A token that lets up to three agents create (or join) a private location
# named "dc-east", valid for 48 hours, with auto-update disabled.
resource "watchpoint_agent_enrollment_token" "dc_east" {
  roles            = ["private-location", "infrastructure-agent"]
  location_name    = "dc-east"
  max_uses         = 3
  expires_in_hours = 48
  auto_update      = false
}

# The token and the install commands that embed it exist only in state and in
# these outputs. Mark any output that carries them as sensitive.
output "dc_east_enrollment_token" {
  value     = watchpoint_agent_enrollment_token.dc_east.token
  sensitive = true
}

output "dc_east_linux_install" {
  value     = watchpoint_agent_enrollment_token.dc_east.install_commands["linux"]
  sensitive = true
}
