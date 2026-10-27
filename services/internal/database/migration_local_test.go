//go:build !integration

package database

import "testing"

// Configured backend evidence is selected by the integration build tag.
func testConfiguredMigrationBaseline(t *testing.T) {}
