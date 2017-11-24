package main

import (
	"log"

	"golang.org/x/sync/errgroup"

	"./checker"
	"./config"
)

var (
	// Version is the version of pull-request-checker
	Version = "0.1.2"
)

func main() {
	var err error
	checker.SetVersion(Version)

	conf, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("error: %v", err)
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

	if err = g.Wait(); err != nil {
		checker.LogError.Fatal(err)
	}
}
