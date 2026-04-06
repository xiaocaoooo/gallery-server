package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/xiaocaoooo/gallery-server/internal/config"
)

var defaultTokenHeaders = []string{"X-Read-Token", "X-Write-Token", "X-API-Token"}

func RequireReadAuth(cfg config.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(cfg.ReadToken) == "" {
			c.Next()
			return
		}

		token := ExtractToken(c.Request, "X-Read-Token", "X-Write-Token", "X-API-Token")
		if !AllowsRead(token, cfg) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func RequireWriteAuth(cfg config.AuthConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.TrimSpace(cfg.WriteToken) == "" {
			c.Next()
			return
		}

		token := ExtractToken(c.Request, "X-Write-Token", "X-API-Token")
		if !AllowsWrite(token, cfg) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		c.Next()
	}
}

func ExtractToken(r *http.Request, preferredHeaders ...string) string {
	if r == nil {
		return ""
	}
	if auth := strings.TrimSpace(r.Header.Get("Authorization")); auth != "" {
		parts := strings.SplitN(auth, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			return strings.TrimSpace(parts[1])
		}
	}

	headers := make([]string, 0, len(preferredHeaders)+len(defaultTokenHeaders))
	headers = append(headers, preferredHeaders...)
	for _, header := range defaultTokenHeaders {
		seen := false
		for _, preferred := range preferredHeaders {
			if strings.EqualFold(preferred, header) {
				seen = true
				break
			}
		}
		if !seen {
			headers = append(headers, header)
		}
	}

	for _, header := range headers {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}

	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func AllowsRead(token string, cfg config.AuthConfig) bool {
	if strings.TrimSpace(cfg.ReadToken) == "" {
		return true
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	if token == cfg.ReadToken {
		return true
	}
	if strings.TrimSpace(cfg.WriteToken) != "" && token == cfg.WriteToken {
		return true
	}
	return false
}

func AllowsWrite(token string, cfg config.AuthConfig) bool {
	if strings.TrimSpace(cfg.WriteToken) == "" {
		return true
	}
	token = strings.TrimSpace(token)
	return token != "" && token == cfg.WriteToken
}
