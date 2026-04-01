// Package middleware provides Gin middleware for logging, recovery, and request IDs.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const headerRequestID = "X-Request-ID"

// RequestID injects a unique request ID into every request context and response header.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(headerRequestID)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set("request_id", id)
		c.Header(headerRequestID, id)
		c.Next()
	}
}

// Logger logs each request with structured slog output.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()
		reqID, _ := c.Get("request_id")

		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		} else if status >= http.StatusBadRequest {
			level = slog.LevelWarn
		}

		slog.Log(
			c.Request.Context(),
			level,
			"request",
			"method", c.Request.Method,
			"path", path,
			"query", query,
			"status", status,
			"duration", duration,
			"ip", c.ClientIP(),
			"request_id", reqID,
		)
	}
}

// Recovery returns a 500 with a JSON error body on panics, logging the error.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, err any) {
		reqID, _ := c.Get("request_id")
		slog.Error(
			"panic recovered",
			"error", err,
			"request_id", reqID,
			"path", c.Request.URL.Path,
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"error":      "internal server error",
			"request_id": reqID,
		})
	})
}
