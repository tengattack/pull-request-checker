package checker

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	// "github.com/gin-gonic/gin/binding"
	api "gopkg.in/appleboy/gin-status-api.v1"
)

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
			"version": GetVersion(),
		},
	})
}

func rootHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"info": "Welcome to pull request checker server.",
	})
}

func routerEngine() *gin.Engine {
	// set server mode
	gin.SetMode(Conf.API.Mode)

	r := gin.New()

	// Global middleware
	r.Use(gin.Logger())
	r.Use(gin.Recovery())
	r.Use(VersionMiddleware())
	r.Use(LogMiddleware())
	r.Use(StatMiddleware())

	r.GET("/api/stat/go", api.StatusHandler)
	r.GET("/api/stat/sys", sysStatsHandler)
	r.POST(Conf.API.WebHookURI, webhookHandler)
	// r.GET("/api/stat/app", appStatusHandler)
	r.GET("/version", versionHandler)
	r.GET("/", rootHandler)

	return r
}

// RunHTTPServer provide run http or https protocol.
func RunHTTPServer() (err error) {
	if !Conf.API.Enabled {
		LogAccess.Debug("HTTPD server is disabled.")
		return nil
	}

	LogAccess.Infof("HTTPD server is running on %s:%d.", Conf.API.Address, Conf.API.Port)
	/* if Conf.Core.AutoTLS.Enabled {
		s := autoTLSServer()
		err = s.ListenAndServeTLS("", "")
	} else if Conf.Core.SSL && Conf.Core.CertPath != "" && Conf.Core.KeyPath != "" {
		err = http.ListenAndServeTLS(Conf.Core.Address+":"+Conf.Core.Port, Conf.Core.CertPath, Conf.Core.KeyPath, routerEngine())
	} else { */
	err = http.ListenAndServe(fmt.Sprintf("%s:%d", Conf.API.Address, Conf.API.Port), routerEngine())
	// }

	return err
}
