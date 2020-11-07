package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tengattack/unified-ci/checker"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/config"
	"github.com/tengattack/unified-ci/store"
	"github.com/tengattack/unified-ci/util"
	"golang.org/x/net/proxy"
	"golang.org/x/sync/errgroup"
)

var (
	// Version is the version of unified-ci
	Version = "0.2.3-dev"
)

func main() {
	common.SetVersion(Version)
	configPath := flag.String("config", "", "config file")
	workerMode := flag.Bool("worker", false, "running in worker mode")
	showHelp := flag.Bool("help", false, "show help message")
	showVerbose := flag.Bool("verbose", false, "show verbose debug log")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showHelp {
		fmt.Printf(common.UserAgent() + "\n\n")
		flag.Usage()
		return
	}
	if *showVersion {
		fmt.Printf(common.UserAgent() + "\n")
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
	common.Conf = conf

	if err = common.InitLog(conf); err != nil {
		log.Fatalf("error: %v", err)
	}

	var tr http.RoundTripper
	if common.Conf.Core.Socks5Proxy != "" {
		dialSocksProxy, err := proxy.SOCKS5("tcp", common.Conf.Core.Socks5Proxy, nil, proxy.Direct)
		if err != nil {
			msg := "Setup proxy failed: " + err.Error()
			err = errors.New(msg)
			log.Fatalf("error: %v", err)
		}
		tr = &http.Transport{Dial: dialSocksProxy.Dial}
	}

	if err = util.InitJWTClient(conf.GitHub.AppID, conf.GitHub.PrivateKey, tr); err != nil {
		log.Fatalf("error: %v", err)
	}

	if err = store.Init(conf.Core.DBFile); err != nil {
		log.Fatalf("error: %v", err)
	}
	defer store.Deinit()

	if !*workerMode {
		if err = checker.InitMessageQueue(); err != nil {
			log.Fatalf("error: %v", err)
		}
	}

	parent, cancel := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(parent)

	leave := make(chan struct{})
	go func() {
		if *workerMode {
			g.Go(func() error {
				// Start message subscription
				checker.StartWorkerMessageSubscription(ctx)
				return nil
			})
		} else {
			if common.Conf.Core.EnableRetries {
				g.Go(func() error {
					// Start error message retries
					checker.RetryErrorMessages(ctx)
					return nil
				})
			}
			g.Go(func() error {
				// Run local repo watcher
				return checker.WatchServerWorkerRepo(ctx)
			})
			/*g.Go(func() error {
				// Start message subscription
				checker.StartMessageSubscription(ctx)
				return nil
			})
			g.Go(func() error {
				// Run local repo watcher
				return checker.WatchLocalRepo(ctx)
			})*/
		}

		g.Go(func() error {
			// Run httpd server
			return checker.RunHTTPServer(*workerMode)
		})

		if err = g.Wait(); err != nil {
			common.LogError.Error(err)
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
		common.LogError.Errorf("Error in ShutdownHTTPServer: %v\n", err)
	}

	select {
	case <-leave:
	case <-time.After(60 * time.Second):
		common.LogAccess.Info("Waiting for leave times out.")
	}
}
