// occccad-dev checks configured resources without migrating or deleting data.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/occccad/occccad/internal/devtool"
	"os"
	"strings"
	"time"

	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

func diagnoseResources(ctx context.Context, configuration config.Config) []check {
	checks := []check{}
	add := func(name, detail string, err error) {
		status := "PASS"
		if err != nil {
			status, detail = "ENVIRONMENT_BLOCKED", config.Redact(err.Error())
		}
		checks = append(checks, check{Name: name, Status: status, Detail: detail})
	}
	// Opening a missing SQLite file would create another empty database during
	// a readiness check. Require normal startup/migration to prepare it first.
	var db *database.Pool
	var err error
	if strings.HasPrefix(configuration.DatabaseURL, "sqlite:") {
		_, err = os.Stat(strings.TrimPrefix(configuration.DatabaseURL, "sqlite:"))
	}
	if err == nil {
		db, err = database.Open(ctx, configuration.DatabaseURL)
	}
	if err == nil {
		defer db.Close()
		var one int
		err = db.QueryRow(ctx, "SELECT 1").Scan(&one)
	}
	add("database", "configured database accepts queries", err)
	if err == nil {
		var count int
		err = db.QueryRow(ctx, "SELECT count(*) FROM occccad.schema_migrations").Scan(&count)
		if err == nil && count == 0 {
			err = fmt.Errorf("database has no applied migrations; run mise run dev")
		}
		add("schema", "migration records present; actual migration validation remains enforced at startup", err)
	}
	if strings.EqualFold(os.Getenv("OCCCCAD_ARTIFACT_BACKEND"), "S3") {
		_, _, err = artifact.OpenConfigured(ctx, configuration.DataDirectory)
		add("artifact-store", "configured S3 bucket is reachable", err)
	} else {
		var info os.FileInfo
		info, err = os.Stat(configuration.DataDirectory)
		if err == nil && !info.IsDir() {
			err = fmt.Errorf("configured data path is not a directory")
		}
		add("artifact-store", "configured LOCAL directory exists", err)
	}
	return checks
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "doctor:resources" {
		resourceMain()
		return
	}
	if err := devtool.Main(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, config.Redact(err.Error()))
		os.Exit(1)
	}
}

func resourceMain() {
	if len(os.Args) != 2 || os.Args[1] != "doctor:resources" {
		fmt.Fprintln(os.Stderr, "usage: occccad-dev doctor")
		os.Exit(1)
	}
	if _, err := config.LoadEnvironment(); err != nil {
		fmt.Fprintln(os.Stderr, "cannot load development environment")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	checks := diagnoseResources(ctx, config.Load())
	if err := json.NewEncoder(os.Stdout).Encode(checks); err != nil {
		os.Exit(1)
	}
	for _, result := range checks {
		if result.Status != "PASS" {
			os.Exit(2)
		}
	}
}
