// occccad-3dreplay replays a numerical assembly fixture through a Worker or Router.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/occccad/occccad/internal/geometry"
)

func main() {
	address := flag.String("worker", "127.0.0.1:51001", "Geometry Worker or Router address")
	output := flag.String("out", "", "output file (default stdout)")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: occccad-3dreplay [-worker address] [-out file] input.3dreplay")
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0))
	must(err)
	client, err := geometry.Open(*address)
	must(err)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := client.ReplayAssembly(ctx, data)
	must(err)
	if *output != "" {
		must(os.WriteFile(*output, result, 0600))
	} else {
		fmt.Println(string(result))
	}
}
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
