package database

import (
	"fmt"
	"os"
	"testing"

	"github.com/occccad/occccad/internal/config"
)

func TestMain(m *testing.M) {
	if _, err := config.LoadEnvironment(); err != nil {
		fmt.Fprintln(os.Stderr, "test environment:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
