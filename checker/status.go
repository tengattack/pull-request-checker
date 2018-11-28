package checker

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/thoas/stats"
)

// Stats provide response time, status code count, etc.
var Stats = stats.New()

func sysStatsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, Stats.Data())
}

// StatMiddleware response time, status code count, etc.
func StatMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		beginning, recorder := Stats.Begin(c.Writer)
		c.Next()
		Stats.End(beginning, stats.WithRecorder(recorder))
	}
}
