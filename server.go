package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"strings"

	//	"io/fs"

	"net/http"
	"net/url"
	"os"

	"github.com/andfenastari/chatsim/core"
	"github.com/andfenastari/chatsim/shell/api"
	"github.com/andfenastari/chatsim/shell/web"
)

var (
	apiAddr = flag.String("api-addr", ":8000", "API server address.")
	webAddr = flag.String("web-addr", ":8001", "Web interface server address.")

	snapshotPath = flag.String("snapshot", "", "Path of the snapshot to load")
	user         = flag.String("user", "agent", "Web server user.")

	devel    = flag.Bool("devel", false, "Turn on development mode.")
	webhooks = flag.String("webhooks", "", "A comma separated list of '<user>:<url>' values to send webhooks to. Example: 'agent:localhost:900,other:localhost:9001'")
)

func main() {
	flag.CommandLine.Usage = usage
	flag.Parse()

	if *snapshotPath == "" {
		die("error: no 'snapshot' provided.\n")
	}

	var hooks []*api.Webhook
	if *webhooks != "" {
		for _, spec := range strings.Split(*webhooks, ",") {
			user, rawUrl, found := strings.Cut(spec, ":")
			if !found {
				die("Failed to parse webhook '%s'", spec)
			}

			parsedUrl, err := url.Parse(rawUrl)
			if err != nil {
				die("Failed to parse url '%s': %v", rawUrl, err)
			}

			hooks = append(hooks, &api.Webhook{User: user, URL: parsedUrl})
		}
	}

	ctx := context.Background()
	core := core.NewCore(ctx)
	if err := core.LoadSnapshot(*snapshotPath); err != nil {
		log.Print(err)
	}

	fmt.Printf("Starting api server at %s\n", *apiAddr)
	fmt.Printf("Starting web server at %s\n", *webAddr)

	go func() {
		handler := api.NewHandler(core)
		for _, hook := range hooks {
			handler.RegisterWebhook(hook.User, hook.URL)
		}
		server := http.Server{Addr: *apiAddr, Handler: handler}
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("API server failed: %v", err)
		}
	}()

	go func() {
		handler := web.NewHandler(core, *user, *devel, *snapshotPath)
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

func die(msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+msg, args...)
	usage()
	os.Exit(1)
}
