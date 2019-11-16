package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tengattack/unified-ci/checker"
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/store"
	"github.com/tengattack/unified-ci/util"
	"golang.org/x/sync/errgroup"
)

const (
	// Version is the version of unified-ci
	Version = "0.1.3"
)

func main() {
	checker.SetVersion(Version)
	configPath := flag.String("config", "", "config file")
	showHelp := flag.Bool("help", false, "show help message")
	showVerbose := flag.Bool("verbose", false, "show verbose debug log")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showHelp {
		fmt.Printf(checker.UserAgent() + "\n\n")
		flag.Usage()
		return
	}
	if *showVersion {
		fmt.Printf(checker.UserAgent() + "\n\n")
		return
	}
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "Please specify a config file")
		flag.Usage()
		os.Exit(1)
	}

	conf, err := config.LoadConfig(*configPath)
	if err != nil {
		panic(err)
	}
	if *showVerbose {
		conf.Log.AccessLevel = "debug"
		conf.Log.ErrorLevel = "debug"
	}

	// set default parameters.
	checker.Conf = conf

	if err = checker.InitLog(conf); err != nil {
		log.Fatalf("error: %v", err)
	}

	if err = util.InitJWTClient(conf.GitHub.AppID, conf.GitHub.PrivateKey); err != nil {
		log.Fatalf("error: %v", err)
	}

	if err = store.Init(conf.Core.DBFile); err != nil {
		log.Fatalf("error: %v", err)
	}
	defer store.Deinit()

	if err = checker.InitMessageQueue(); err != nil {
		log.Fatalf("error: %v", err)
	}

	parent, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(parent)

	leave := make(chan struct{})
	go func() {
		if checker.Conf.Core.EnableRetries {
			g.Go(func() error {
				// Start error message retries
				checker.RetryErrorMessages(ctx)
				return nil
			})
		}

		g.Go(func() error {
			// Start message subscription
			checker.StartMessageSubscription(ctx)
			return nil
		})

		g.Go(func() error {
			// Run httpd server
			return checker.RunHTTPServer()
		})

		g.Go(func() error {
			// Run local repo watcher
			return checker.WatchLocalRepo(ctx)
		})

		if err = g.Wait(); err != nil {
			checker.LogError.Error(err)
		}
		close(leave)
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-shutdown:
	case <-ctx.Done():
	}

	cancel()
	err = checker.ShutdownHTTPServer(60 * time.Second)
	if err != nil {
		checker.LogError.Errorf("Error in ShutdownHTTPServer: %v\n", err)
	}

	select {
	case <-leave:
	case <-time.After(60 * time.Second):
		checker.LogAccess.Info("Waiting for leave times out.")
	}
}
