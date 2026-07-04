package migrations

import _ "embed"

// Current is bumped when schema_vN.sql is added. Deploy runs pending files in order.
const Current = 1

//go:embed schema_v1.sql
var SchemaV1 string

// Names maps version → DDL file for ops / local bootstrap.
var Names = map[int]string{
	1: "schema_v1.sql",
}
