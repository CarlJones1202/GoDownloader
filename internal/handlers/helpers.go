// Package handlers provides shared helpers for HTTP handlers.
package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/carlj/godownload/internal/database"
	"github.com/gin-gonic/gin"
)

// respondOK writes a 200 JSON response.
func respondOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

// respondCreated writes a 201 JSON response.
func respondCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

// respondNoContent writes a 204 response with no body.
func respondNoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// respondError writes a JSON error response with the given status code.
func respondError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

// parseIDParam parses the ":id" path parameter as int64.
// Returns false and writes a 400 response if parsing fails.
func parseIDParam(c *gin.Context) (int64, bool) {
	return parseIntParam(c, "id")
}

// parseIntParam parses a named path parameter as int64.
func parseIntParam(c *gin.Context, name string) (int64, bool) {
	val, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid "+name+": must be an integer")
		return 0, false
	}
	return val, true
}

// handleDBError maps database errors to appropriate HTTP responses.
func handleDBError(c *gin.Context, err error) {
	if errors.Is(err, database.ErrNotFound) {
		respondError(c, http.StatusNotFound, "not found")
		return
	}
	respondError(c, http.StatusInternalServerError, "internal server error")
}

// paginationParams extracts limit and offset from query params with defaults.
func paginationParams(c *gin.Context) (limit, offset int) {
	limit = 50
	offset = 0

	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
