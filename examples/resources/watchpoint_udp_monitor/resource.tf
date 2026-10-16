# Send a raw DNS query for example.com (type A) and expect a reply.
resource "watchpoint_udp_monitor" "resolver" {
  name           = "Internal resolver"
  host           = "ns1.example.com"
  port           = 53
  payload        = "abcd01000001000000000000076578616d706c6503636f6d0000010001"
  payload_is_hex = true
  expect_regex   = ".+"
}
