package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"

	//	"io/fs"

	"net/http"
	"os"

	"github.com/andfenastari/chatsim/core"
	"github.com/andfenastari/chatsim/shell/api"
	"github.com/andfenastari/chatsim/shell/web"
)

var (
	apiAddr = flag.String("api-addr", ":8000", "API server address. Defaults to ':8000'")
	webAddr = flag.String("web-addr", ":8001", "Web interface server address. Defaults to ':8001'")

	snapshotPath = flag.String("snapshot", "", "Path of the snapshot to load")
	user         = flag.String("user", "+00", "Web server user. Defaults to '+00'")

	devel = flag.Bool("devel", false, "Turn on development mode.")
)

func main() {
	flag.CommandLine.Usage = usage
	flag.Parse()

	ctx := context.Background()
	core := core.NewCore(ctx)

	fmt.Printf("Starting api server at %s\n", *apiAddr)
	fmt.Printf("Starting web server at %s\n", *webAddr)

	go func() {
		handler := api.NewHandler(core)
		server := http.Server{Addr: *apiAddr, Handler: handler}
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	go func() {
		handler := web.NewHandler(core, *user, *devel)
		server := http.Server{Addr: *webAddr, Handler: handler}
		server.ListenAndServe()
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Web server failed: %v", err)
		}
	}()

	for {
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "USAGE: %s [flag]...\nAvailable flags:\n", os.Args[0])
	flag.PrintDefaults()
}
