//go:build integration

package database

import (
	"fmt"
	"github.com/occccad/occccad/internal/config"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if _, e := config.LoadEnvironment(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
