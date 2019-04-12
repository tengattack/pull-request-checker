package checker

import "github.com/gin-gonic/gin"

// Version is the version of unified-ci
const Version = "0.1.2"

// VersionMiddleware : add version on header.
func VersionMiddleware() gin.HandlerFunc {
	// Set out header value for each response
	return func(c *gin.Context) {
		c.Header("X-DRONE-Version", Version)
		c.Next()
	}
}
