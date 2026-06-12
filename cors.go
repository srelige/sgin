package sgin

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	headerOrigin                        = "Origin"
	headerVary                          = "Vary"
	headerAccessControlAllowOrigin      = "Access-Control-Allow-Origin"
	headerAccessControlAllowMethods     = "Access-Control-Allow-Methods"
	headerAccessControlAllowHeaders     = "Access-Control-Allow-Headers"
	headerAccessControlExposeHeaders    = "Access-Control-Expose-Headers"
	headerAccessControlAllowCredentials = "Access-Control-Allow-Credentials"
	headerAccessControlMaxAge           = "Access-Control-Max-Age"
	headerAccessControlRequestMethod    = "Access-Control-Request-Method"
	headerAccessControlRequestHeaders   = "Access-Control-Request-Headers"
)

func (a *App) corsMiddleware() gin.HandlerFunc {
	cfg := a.config.CORS
	allowOrigins := nonEmptyStrings(cfg.AllowOrigins)
	allowMethods := normalizeHeaderValues(cfg.AllowMethods, true)
	allowHeaders := normalizeHeaderValues(cfg.AllowHeaders, false)
	exposeHeaders := normalizeHeaderValues(cfg.ExposeHeaders, false)
	allowAllOrigins := containsString(allowOrigins, "*")
	allowedOrigins := make(map[string]struct{}, len(allowOrigins))
	for _, origin := range allowOrigins {
		allowedOrigins[origin] = struct{}{}
	}

	maxAgeSeconds := ""
	if cfg.MaxAge != "" {
		if maxAge, err := time.ParseDuration(cfg.MaxAge); err == nil {
			maxAgeSeconds = strconv.FormatInt(int64(maxAge/time.Second), 10)
		}
	}

	return func(c *gin.Context) {
		origin := strings.TrimSpace(c.GetHeader(headerOrigin))
		if origin == "" {
			c.Next()
			return
		}

		if !allowAllOrigins {
			c.Header(headerVary, appendVary(c.Writer.Header().Get(headerVary), headerOrigin))
		}

		if !allowAllOrigins {
			if _, ok := allowedOrigins[origin]; !ok {
				if c.Request.Method == http.MethodOptions && c.GetHeader(headerAccessControlRequestMethod) != "" {
					c.AbortWithStatus(http.StatusForbidden)
					return
				}
				c.Next()
				return
			}
		}

		if allowAllOrigins {
			c.Header(headerAccessControlAllowOrigin, "*")
		} else {
			c.Header(headerAccessControlAllowOrigin, origin)
		}
		if cfg.AllowCredentials {
			c.Header(headerAccessControlAllowCredentials, "true")
		}
		if len(exposeHeaders) > 0 {
			c.Header(headerAccessControlExposeHeaders, strings.Join(exposeHeaders, ", "))
		}

		if c.Request.Method == http.MethodOptions && c.GetHeader(headerAccessControlRequestMethod) != "" {
			if len(allowMethods) > 0 {
				c.Header(headerAccessControlAllowMethods, strings.Join(allowMethods, ", "))
			}
			if len(allowHeaders) > 0 {
				c.Header(headerAccessControlAllowHeaders, strings.Join(allowHeaders, ", "))
			} else if requestedHeaders := strings.TrimSpace(c.GetHeader(headerAccessControlRequestHeaders)); requestedHeaders != "" {
				c.Header(headerAccessControlAllowHeaders, requestedHeaders)
			}
			if maxAgeSeconds != "" {
				c.Header(headerAccessControlMaxAge, maxAgeSeconds)
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func normalizeHeaderValues(values []string, upper bool) []string {
	cleaned := nonEmptyStrings(values)
	for i, value := range cleaned {
		if upper {
			cleaned[i] = strings.ToUpper(value)
		}
	}
	return cleaned
}

func appendVary(current string, value string) string {
	if current == "" {
		return value
	}
	for _, part := range strings.Split(current, ",") {
		if strings.EqualFold(strings.TrimSpace(part), value) {
			return current
		}
	}
	return current + ", " + value
}
