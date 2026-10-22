---
page_title: "Managed monitors"
subcategory: ""
description: |-
  How Watchpoint records monitors managed by Terraform, locks edits made elsewhere and reports drift.
---

# Managed monitors

Every monitor resource in this provider tells Watchpoint that Terraform manages
the monitor. Watchpoint then:

- shows a **Managed by Terraform** badge with the workspace on the monitor list
  and detail pages, and warns anyone editing the monitor in the portal that the
  next `terraform apply` overwrites their change;
- on the Scale plan and above, lets an organization administrator lock edits to
  managed monitors made outside Terraform (portal, API keys without the provider,
  bulk edits and imports); and
- on the Scale plan and above, lists **Terraform drift**: managed monitors
  changed outside Terraform since the provider last applied them, with the
  changed fields and who changed them.

## How a monitor becomes managed

The provider sends three headers on every create, update, delete and SLA attach
of a monitor:

| Header | Value |
|---|---|
| `X-Watchpoint-Managed-By` | `terraform` |
| `X-Watchpoint-Terraform-Workspace` | the provider `workspace` argument, else `TF_WORKSPACE`, else `default` |
| `X-Watchpoint-Terraform-Address` | the resource type, for example `watchpoint_http_monitor` |

Watchpoint honours the headers only on API-key requests, so a portal session
cannot mark a monitor as managed. Terraform does not pass full resource
addresses to providers, so the address is the resource type; the monitor id is
the stable key.

`terraform import` sends one empty update so the imported monitor is recorded
straight away. If that update fails (for example, the API key lacks
`monitors.update`), the import still succeeds with a warning and the next apply
that changes the monitor records it.

Destroying a resource removes the marker. Removing a resource from state with
`terraform state rm` leaves the marker in place; edit the monitor once through
the provider, or delete and re-create it, to change that.

## Workspaces

```terraform
provider "watchpoint" {
  workspace = "production"
}
```

Set `workspace` when one organization is managed from several configurations so
the badge and the drift view say which one owns a monitor.

## Locking edits made outside Terraform

An administrator with the `settings.manage` permission turns the lock on under
**Monitoring > Terraform drift**. While it is on, updates, deletes and history
restores of a managed monitor that do not come from the provider are refused
with HTTP 409 and the code `monitor_managed_by_terraform`; bulk edits report the
monitor as skipped. Creating monitors is never locked. If the organization moves
to a plan without the feature, the lock stops applying and nothing is deleted.
