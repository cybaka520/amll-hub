package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/amll-dev/amll-hub/backend/internal/pkg"
	"github.com/gin-gonic/gin"
	logrus "github.com/sirupsen/logrus"
)

// Recovery panic 恢复中间件
func Recovery() gin.HandlerFunc {
	log := logrus.StandardLogger()
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.WithFields(logrus.Fields{
					"request_id": GetRequestID(c),
					"error":      rec,
					"stack":      string(debug.Stack()),
				}).Error("panic recovered")
				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, pkg.Response{
						Code:    500,
						Message: "internal server error",
					})
				}
			}
		}()
		c.Next()
	}
}
