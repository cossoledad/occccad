// Artifact administration never deletes originals or rewrites model history.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/occccad/occccad/internal/artifact"
	"github.com/occccad/occccad/internal/config"
	"github.com/occccad/occccad/internal/database"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	initialize := flag.Bool("init-bucket", false, "create the configured bucket if absent")
	migrate := flag.Bool("migrate-local", false, "copy LOCAL objects to configured backend; keep originals")
	verify := flag.Bool("verify-target", false, "verify all READY objects exist in configured backend with matching size and SHA-256")
	flag.Parse()
	if _, err := config.LoadProjectEnv(); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *initialize {
		secure := true
		var err error
		if raw := os.Getenv("OCCCCAD_S3_SECURE"); raw != "" {
			secure, err = strconv.ParseBool(raw)
			if err != nil {
				return err
			}
		}
		client, err := minio.New(os.Getenv("OCCCCAD_S3_ENDPOINT"), &minio.Options{Creds: credentials.NewStaticV4(os.Getenv("OCCCCAD_S3_ACCESS_KEY"), os.Getenv("OCCCCAD_S3_SECRET_KEY"), ""), Secure: secure, Region: os.Getenv("OCCCCAD_S3_REGION"), BucketLookup: minio.BucketLookupPath, MaxRetries: 1})
		if err != nil {
			return err
		}
		bucket := os.Getenv("OCCCCAD_S3_BUCKET")
		exists, err := client.BucketExists(ctx, bucket)
		if err != nil {
			return err
		}
		if !exists {
			if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: os.Getenv("OCCCCAD_S3_REGION")}); err != nil {
				return err
			}
		}
		fmt.Println("configured S3 bucket is ready")
	}
	if *migrate || *verify {
		c := config.Load()
		store, local, err := artifact.OpenConfigured(ctx, c.DataDirectory)
		if err != nil {
			return err
		}
		db, err := database.Open(ctx, c.DatabaseURL)
		if err != nil {
			return err
		}
		defer db.Close()
		if err := database.Migrate(ctx, db); err != nil {
			return err
		}
		service := artifact.NewService(db, store, local)
		if *migrate {
			embedded, err := service.MigrateEmbedded(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("materialized %d embedded artifacts; database originals retained\n", embedded)
			count, err := service.MigrateLocal(ctx)
			fmt.Printf("migrated %d artifacts; local originals retained\n", count)
			if err != nil {
				return err
			}
		}
		if *verify {
			count, err := service.VerifyTarget(ctx)
			fmt.Printf("verified %d artifacts in %s\n", count, store.Backend())
			return err
		}
	}
	if !*initialize && !*migrate && !*verify {
		return fmt.Errorf("specify --init-bucket, --migrate-local or --verify-target")
	}
	return nil
}
