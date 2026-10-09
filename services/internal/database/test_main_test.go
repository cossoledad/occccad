package database

import (
	"fmt"
	"github.com/occccad/occccad/internal/config"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if err := config.LoadTestEnvironment(); err != nil {
		fmt.Fprintln(os.Stderr, "test environment:", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
