package cli

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/visionik/sogcli/internal/carddav"
)

func captureContactOutput(t *testing.T, output func() error) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "output")
	require.NoError(t, err)
	defer file.Close()
	previous := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = previous }()
	require.NoError(t, output())
	_, err = file.Seek(0, io.SeekStart)
	require.NoError(t, err)
	data, err := io.ReadAll(file)
	require.NoError(t, err)
	return string(data)
}

func TestOutputContactsJSONEscapingAndDetails(t *testing.T) {
	contacts := []carddav.Contact{
		{
			UID: "uid\"\\first", FullName: "Fixture \"Hall\"\nSecond line\\path",
			Emails: []string{"first@example.invalid", "second@example.invalid"},
			Phones: []string{"+1-206-555-0100", "+1-206-555-0101"},
			Org:    "Fixtures & <Tests>", FirstName: "Fixture", LastName: "Contact",
			Title: "Test\trole", Note: "Original notes\nCoordinates: synthetic only",
			Birthday: "2000-01-15", Addresses: []string{"123 \"Example\" Road\nExampleville", "456 Other Road"},
			URL: "https://example.invalid/?a=1&b=2", ETag: "\"fixture-etag\"",
		},
		{UID: "second", FullName: "Second contact"},
	}
	output := captureContactOutput(t, func() error { return outputContactsJSON(contacts) })
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	require.Len(t, lines, len(contacts), "each contact must occupy exactly one JSON line")
	for i, line := range lines {
		var got struct {
			UID       string   `json:"uid"`
			FullName  string   `json:"full_name"`
			Emails    string   `json:"emails"`
			Phones    string   `json:"phones"`
			Org       string   `json:"org"`
			FirstName string   `json:"first_name"`
			LastName  string   `json:"last_name"`
			Title     string   `json:"title"`
			Note      string   `json:"note"`
			Birthday  string   `json:"birthday"`
			Addresses []string `json:"addresses"`
			URL       string   `json:"url"`
			ETag      string   `json:"etag"`
		}
		require.NoError(t, json.Unmarshal([]byte(line), &got))
		require.Equal(t, contacts[i].UID, got.UID)
		require.Equal(t, contacts[i].FullName, got.FullName)
		require.Equal(t, strings.Join(contacts[i].Emails, ","), got.Emails)
		require.Equal(t, strings.Join(contacts[i].Phones, ","), got.Phones)
		require.Equal(t, contacts[i].Org, got.Org)
		require.Equal(t, contacts[i].FirstName, got.FirstName)
		require.Equal(t, contacts[i].LastName, got.LastName)
		require.Equal(t, contacts[i].Title, got.Title)
		require.Equal(t, contacts[i].Note, got.Note)
		require.Equal(t, contacts[i].Birthday, got.Birthday)
		require.Equal(t, contacts[i].Addresses, got.Addresses)
		require.Equal(t, contacts[i].URL, got.URL)
		require.Equal(t, contacts[i].ETag, got.ETag)
	}
	require.Contains(t, lines[0], `"org":"Fixtures & <Tests>"`)
}

func TestOutputContactsJSONLegacyFields(t *testing.T) {
	output := captureContactOutput(t, func() error {
		return outputContactsJSON([]carddav.Contact{{UID: "fixture", FullName: "Fixture Hall", Emails: []string{"a@example.invalid", "b@example.invalid"}, Phones: []string{"111", "222"}}})
	})
	require.Equal(t, "{\"uid\":\"fixture\",\"full_name\":\"Fixture Hall\",\"emails\":\"a@example.invalid,b@example.invalid\",\"phones\":\"111,222\",\"org\":\"\"}\n", output)
	empty := captureContactOutput(t, func() error { return outputContactsJSON([]carddav.Contact{{}}) })
	require.Equal(t, "{\"uid\":\"\",\"full_name\":\"\",\"emails\":\"\",\"phones\":\"\",\"org\":\"\"}\n", empty)
}

func TestOutputContactsJSONReadFields(t *testing.T) {
	contact := carddav.Contact{UID: "fixture", FullName: "Fixture Hall", Note: "Keep the original notes", Addresses: []string{"123 Example Road"}, FirstName: "Fixture", LastName: "Contact", Title: "Role", Birthday: "2000-01-15", URL: "https://example.invalid", ETag: "fixture-v1"}
	output := captureContactOutput(t, func() error { return outputContactsJSON([]carddav.Contact{contact}) })
	var got map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(output), &got))
	for _, field := range []string{"uid", "full_name", "emails", "phones", "org", "first_name", "last_name", "title", "note", "birthday", "addresses", "url", "etag"} {
		require.Contains(t, got, field)
	}
	var addresses []string
	require.NoError(t, json.Unmarshal(got["addresses"], &addresses))
	require.Equal(t, contact.Addresses, addresses)
}

func TestOutputAddressBooksJSONEscaping(t *testing.T) {
	books := []carddav.AddressBook{
		{Path: "/books/quoted\"path/", Name: "Fixture \"Book\"", Description: "First line\nSecond\\line & <text>"},
		{Path: "/books/empty/", Name: "Empty description"},
	}
	output := captureContactOutput(t, func() error { return outputAddressBooksJSON(books) })
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	require.Len(t, lines, len(books))
	for i, line := range lines {
		var got carddav.AddressBook
		require.NoError(t, json.Unmarshal([]byte(line), &got))
		require.Equal(t, books[i], got)
	}
	require.Equal(t, `{"path":"/books/empty/","name":"Empty description","description":""}`, lines[1])
	require.Contains(t, lines[0], "& <text>")
}

func TestContactJSONEmptyLists(t *testing.T) {
	tests := []struct {
		name   string
		output func() error
	}{
		{"contacts", func() error { return outputContactsJSON(nil) }},
		{"address books", func() error { return outputAddressBooksJSON(nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Empty(t, captureContactOutput(t, tt.output))
		})
	}
}

func TestContactJSONWriteFailure(t *testing.T) {
	tests := []struct {
		name   string
		output func() error
	}{
		{"contacts", func() error { return outputContactsJSON([]carddav.Contact{{UID: "fixture"}}) }},
		{"address books", func() error { return outputAddressBooksJSON([]carddav.AddressBook{{Path: "/books/fixture/"}}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := os.CreateTemp(t.TempDir(), "closed-output")
			require.NoError(t, err)
			require.NoError(t, file.Close())
			previous := os.Stdout
			os.Stdout = file
			defer func() { os.Stdout = previous }()
			require.Error(t, tt.output())
		})
	}
}
