package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"golang.org/x/sync/errgroup"

	"./checker"
	"./config"
)

var (
	// Version is the version of unified-ci
	Version = "0.1.2"
)

func main() {
	checker.SetVersion(Version)

	configPath := flag.String("config", "", "config file")
	showHelp := flag.Bool("help", false, "show help message")
	showVerbose := flag.Bool("verbose", false, "show verbose debug log")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showHelp {
		fmt.Printf("unified-ci %s\n\n", Version)
		flag.Usage()
		return
	}
	if *showVersion {
		fmt.Printf("unified-ci %s\n", Version)
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

	if err = checker.InitLog(); err != nil {
		log.Fatalf("error: %v", err)
	}

	if err = checker.InitMessageQueue(); err != nil {
		return
	}

	var g errgroup.Group

	if checker.Conf.Core.EnableRetries {
		g.Go(func() error {
			// Start error message retries
			checker.RetryErrorMessages()
			return nil
		})
	}

	g.Go(func() error {
		// Start message subscription
		checker.StartMessageSubscription()
		return nil
	})

	g.Go(func() error {
		// Run httpd server
		return checker.RunHTTPServer()
	})

	g.Go(func() error {
		// Run local repo watcher
		return checker.WatchLocalRepo()
	})

	if err = g.Wait(); err != nil {
		checker.LogError.Fatal(err)
	}
}
