package checker

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tengattack/unified-ci/common"
	"github.com/tengattack/unified-ci/log"
	api "gopkg.in/appleboy/gin-status-api.v1"
)

// VersionMiddleware : add version on header.
func VersionMiddleware() gin.HandlerFunc {
	// Set out header value for each response
	return func(c *gin.Context) {
		c.Header("X-DRONE-Version", common.GetVersion())
		c.Next()
	}
}

func abortWithError(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, gin.H{
		"code": code,
		"info": message,
	})
}

func versionHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"info": gin.H{
			"version": common.GetVersion(),
		},
	})
}

func rootHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"info": "Welcome to pull request checker server.",
	})
}

func routerEngine(mode Mode) *gin.Engine {
	// set server mode
	gin.SetMode(common.Conf.API.Mode)

	r := gin.New()

	// Global middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(VersionMiddleware())
	r.Use(log.Middleware(common.Conf.Log.Format))
	r.Use(StatMiddleware())

	r.GET("/api/stat/go", api.StatusHandler)
	r.GET("/api/stat/sys", sysStatsHandler)
	switch mode {
	case ModeLocal:
		r.POST(common.Conf.API.WebHookURI, webhookHandler)
	case ModeServer:
		r.POST("/api/worker/join", workerJoinHandler)
		r.POST("/api/worker/request", workerRequestHandler)
		r.POST("/api/worker/jobdone", workerJobDoneHandler)
		r.POST(common.Conf.API.WebHookURI, webhookHandler)
	}
	// r.GET("/api/stat/app", appStatusHandler)
	r.GET("/version", versionHandler)
	r.GET("/badges/:owner/:repo/:type", badgesHandler)
	r.GET("/", rootHandler)

	return r
}

var httpSrv *http.Server

// RunHTTPServer provide run http or https protocol.
func RunHTTPServer(mode Mode) (err error) {
	if !common.Conf.API.Enabled {
		common.LogAccess.Debug("HTTPD server is disabled.")
		return nil
	}

	common.LogAccess.Infof("HTTPD server is running on %s:%d.", common.Conf.API.Address, common.Conf.API.Port)
	/* if common.Conf.Core.AutoTLS.Enabled {
		s := autoTLSServer()
		err = s.ListenAndServeTLS("", "")
	} else if common.Conf.Core.SSL && common.Conf.Core.CertPath != "" && common.Conf.Core.KeyPath != "" {
		err = http.ListenAndServeTLS(common.Conf.Core.Address+":"+common.Conf.Core.Port, common.Conf.Core.CertPath, common.Conf.Core.KeyPath, routerEngine())
	} else { */
	httpSrv = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", common.Conf.API.Address, common.Conf.API.Port),
		Handler: routerEngine(mode),
	}
	err = httpSrv.ListenAndServe()
	// }

	if err != http.ErrServerClosed {
		common.LogError.Errorf("HTTP server ListenAndServe returned error: %v", err)
		return err
	}
	common.LogAccess.Warn("RunHTTPServer canceled.")
	return nil
}

// ShutdownHTTPServer shuts down the http server
func ShutdownHTTPServer(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}
