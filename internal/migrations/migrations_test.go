package migrations_test

import (
	"strings"
	"testing"

	"github.com/example/booking-engine/internal/migrations"
)

func TestSchemaV1Present(t *testing.T) {
	if migrations.Current != 1 {
		t.Fatalf("current=%d", migrations.Current)
	}
	if !strings.Contains(migrations.SchemaV1, "event_streams") {
		t.Fatal("missing event_streams DDL")
	}
	if !strings.Contains(migrations.SchemaV1, "inventory_slots") {
		t.Fatal("missing inventory_slots DDL")
	}
}
