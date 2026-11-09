package caldav

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListEventsRequestsAndReadsExpandedOccurrences(t *testing.T) {
	start := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "REPORT", r.Method)
		assert.Equal(t, "/calendar/", r.URL.Path)
		var query struct {
			Expand *struct {
				Start string `xml:"start,attr"`
				End   string `xml:"end,attr"`
			} `xml:"prop>calendar-data>expand"`
		}
		if err := xml.NewDecoder(r.Body).Decode(&query); !assert.NoError(t, err) {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		assert.NotNil(t, query.Expand, "range queries must request server-side recurrence expansion")
		if query.Expand != nil {
			assert.Equal(t, "20260115T000000Z", query.Expand.Start)
			assert.Equal(t, "20260117T000000Z", query.Expand.End)
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(http.StatusMultiStatus)
		// A CalDAV server returns multiple VEVENT components for an expanded
		// resource, including moved and cancelled exceptions with the same UID.
		fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?>
<d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
<d:response><d:href>/calendar/series.ics</d:href><d:propstat><d:prop>
<d:getetag>"series-etag"</d:getetag><c:calendar-data><![CDATA[BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:weekly@example.invalid
RECURRENCE-ID:20260115T100000Z
SUMMARY:Normal occurrence
DTSTART:20260115T100000Z
DURATION:PT1H
STATUS:CONFIRMED
END:VEVENT
BEGIN:VEVENT
UID:weekly@example.invalid
RECURRENCE-ID:20260116T100000Z
SUMMARY:Moved occurrence
DTSTART:20260116T120000Z
DTEND:20260116T133000Z
STATUS:TENTATIVE
END:VEVENT
BEGIN:VEVENT
UID:weekly@example.invalid
RECURRENCE-ID:20260116T150000Z
SUMMARY:Cancelled occurrence
DTSTART:20260116T150000Z
DURATION:PT30M
STATUS:CANCELLED
END:VEVENT
END:VCALENDAR
]]></c:calendar-data></d:prop><d:status>HTTP/1.1 200 OK</d:status>
</d:propstat></d:response>
<d:response><d:href>/calendar/all-day.ics</d:href><d:propstat><d:prop>
<d:getetag>"all-day-etag"</d:getetag><c:calendar-data><![CDATA[BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:all-day@example.invalid
RECURRENCE-ID;VALUE=DATE:20260116
SUMMARY:All-day occurrence
DTSTART;VALUE=DATE:20260116
DTEND;VALUE=DATE:20260117
END:VEVENT
END:VCALENDAR
]]></c:calendar-data></d:prop><d:status>HTTP/1.1 200 OK</d:status>
</d:propstat></d:response></d:multistatus>`)
	}))
	defer server.Close()
	client, err := Connect(Config{URL: server.URL})
	require.NoError(t, err)
	defer client.Close()
	events, err := client.ListEvents(context.Background(), "/calendar/", start, end)
	require.NoError(t, err)
	require.Len(t, events, 4, "all expanded occurrences must survive parsing")
	require.Equal(t, start.Add(10*time.Hour), events[0].Start)
	require.Equal(t, start.Add(11*time.Hour), events[0].End)
	require.Equal(t, start.AddDate(0, 0, 1).Add(12*time.Hour), events[1].Start)
	require.Equal(t, start.AddDate(0, 0, 1).Add(13*time.Hour+30*time.Minute), events[1].End)
	require.Equal(t, "TENTATIVE", events[1].Status)
	require.Equal(t, "CANCELLED", events[2].Status)
	require.Equal(t, 30*time.Minute, events[2].End.Sub(events[2].Start))
	require.True(t, events[3].AllDay)
	require.Equal(t, "2026-01-16", events[3].Start.Format("2006-01-02"))
	require.Equal(t, "2026-01-17", events[3].End.Format("2006-01-02"))
	for _, event := range events[:3] {
		require.Equal(t, "weekly@example.invalid", event.UID)
		require.Equal(t, "series-etag", event.ETag)
	}
	require.Equal(t, "all-day@example.invalid", events[3].UID)
	require.Equal(t, "all-day-etag", events[3].ETag)
}

func TestListEventsExpansionFailureIsNotEmptyCalendar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "synthetic expansion refusal", http.StatusForbidden)
	}))
	defer server.Close()
	client, err := Connect(Config{URL: server.URL})
	require.NoError(t, err)
	defer client.Close()
	start := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	events, err := client.ListEvents(context.Background(), "/calendar/", start, start.AddDate(0, 0, 1))
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to query calendar")
	require.Nil(t, events)
}
