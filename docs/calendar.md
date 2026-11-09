# Calendar reads

Calendar commands use the selected account's CalDAV configuration. `--account`
(or `SOG_ACCOUNT`) selects the account; otherwise the configured default is used.
The optional argument to `cal list` is a calendar path, not an account email.
`sog cal calendars --json` reports paths available to that account.

## Module map and flow

- [`internal/cli/cal.go`](../internal/cli/cal.go) resolves the account/calendar,
  parses date bounds, applies the requested result limit and formats output.
- [`internal/caldav/client.go`](../internal/caldav/client.go) owns the CalDAV
  query and iCalendar event parsing. Its `ListEvents` uses go-webdav's public
  `CalendarExpandRequest` to ask the server for recurrence occurrences.
- The CalDAV server owns recurrence-rule and exception expansion. The client
  preserves every returned VEVENT instead of selecting only one component from
  a resource. Each occurrence retains its start/end, all-day flag, status and
  resource ETag. Single-resource operations still select the first event.

A list request proceeds from CLI date parsing to a CalDAV REPORT with matching
filter and expansion bounds, then parses all returned occurrences. The CLI
applies `--max` to that occurrence list before writing table or JSONL output.
`today` and `week` use the same list path. A rejected query returns an error;
it does not silently retry without expansion or report an empty calendar.

## Date bounds and complete reads

The query uses an inclusive start and exclusive end. For a full host-local day:

```bash
sog cal list /calendars/example/ --from today --to tomorrow --max 0 --json --no-input
sog cal list /calendars/example/ --from tomorrow --to +2d --max 0 --json --no-input
```

Relative dates are host-local midnights, advanced by calendar days. Explicit
`YYYY-MM-DD` dates are parsed as UTC midnights. `cal list` defaults to at most
50 occurrences; `--max 0` disables that client-side truncation. The server's
response order is preserved.

## JSON output

Event output is one JSON object per line, with string fields escaped by Go's
standard JSON encoder. Quoted titles, backslashes and embedded line breaks do
not break JSONL framing. Existing timestamp formatting remains RFC3339:

```json
{"uid":"example","summary":"Planning","start":"2026-01-15T10:00:00Z","end":"2026-01-15T11:00:00Z","location":"Room A","all_day":false,"status":"CANCELLED"}
```

`status` carries the iCalendar status when present, including `CANCELLED`; it is
omitted when unset. Consumers decide how to use that status. Calendar discovery
also writes one safely encoded object per line, with `path`, `name` and
`description`. Output write failures are returned to the caller.

A successful list with no events prints `No events found.`, including under
`--json`. Consumers must distinguish that empty result from a failed command or
malformed JSON output.
