package persistence

import "fmt"

// oneLineNameSQL is a Postgres expression for assets.OneLineName: control
// characters become spaces, runs of white space one space, ends trimmed.
const oneLineNameSQL = `btrim(regexp_replace(regexp_replace(%s, '[[:cntrl:]]', ' ', 'g'), '\s+', ' ', 'g'))`

// schemaMigrationDeviceNamesOneLine stores every device name as one line. A
// name with a line break, such as "nas01\r\nBcc: x@example.com", was stored as
// typed and printed on two lines; the Hub now refuses such names and stores
// names from agents and connectors as one line, and this turns existing ones
// (and their kept renames) the same way. A name that was only control
// characters becomes the device's ID.
func schemaMigrationDeviceNamesOneLine() schemaMigration {
	clean := func(column string) string { return fmt.Sprintf(oneLineNameSQL, column) }
	return schemaMigration{Version: 111, Name: "device_names_one_line", Statements: []string{
		`UPDATE assets SET name = COALESCE(NULLIF(` + clean("name") + `, ''), id) WHERE name ~ '[[:cntrl:]]'`,
		`UPDATE assets SET metadata = jsonb_set(metadata, '{name_override}', to_jsonb(` + clean("metadata->>'name_override'") + `))
		 WHERE metadata->>'name_override' ~ '[[:cntrl:]]'`,
	}}
}
