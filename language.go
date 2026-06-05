package sgin

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	defaultLanguage       = "en"
	languageContextAppKey = "sgin.app"
)

func defaultMessages() map[string]map[int]string {
	return map[string]map[int]string{
		"en": {
			http.StatusBadRequest:          "Bad Request",
			http.StatusUnauthorized:        "Unauthorized",
			http.StatusForbidden:           "Forbidden",
			http.StatusNotFound:            "Not Found",
			http.StatusInternalServerError: "Internal Server Error",
		},
		"cn": {
			http.StatusBadRequest:          "请求参数错误",
			http.StatusUnauthorized:        "未认证或登录已失效",
			http.StatusForbidden:           "无权访问",
			http.StatusNotFound:            "资源不存在",
			http.StatusInternalServerError: "服务内部错误",
		},
	}
}

func normalizeLanguage(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return defaultLanguage
	}
	return lang
}

func (a *App) Language(lang string) *App {
	if a == nil {
		return nil
	}
	a.languageMu.Lock()
	defer a.languageMu.Unlock()
	a.language = normalizeLanguage(lang)
	return a
}

func (a *App) RegisterLanguage(lang string, messages map[int]string) *App {
	if a == nil {
		return nil
	}
	lang = normalizeLanguage(lang)
	if len(messages) == 0 {
		return a
	}
	copied := make(map[int]string, len(messages))
	for status, message := range messages {
		message = strings.TrimSpace(message)
		if message != "" {
			copied[status] = message
		}
	}
	if len(copied) == 0 {
		return a
	}

	a.languageMu.Lock()
	defer a.languageMu.Unlock()
	if a.messages == nil {
		a.messages = defaultMessages()
	}
	a.messages[lang] = copied
	return a
}

func (a *App) languageMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(languageContextAppKey, a)
		c.Next()
	}
}

func localizedStatusMessage(c *gin.Context, status int) string {
	if app, ok := appFromContext(c); ok {
		return app.message(status)
	}
	return messageFromCatalog(defaultMessages(), defaultLanguage, status)
}

func appFromContext(c *gin.Context) (*App, bool) {
	if c == nil {
		return nil, false
	}
	value, exists := c.Get(languageContextAppKey)
	if !exists {
		return nil, false
	}
	app, ok := value.(*App)
	return app, ok && app != nil
}

func (a *App) message(status int) string {
	if a == nil {
		return messageFromCatalog(defaultMessages(), defaultLanguage, status)
	}
	a.languageMu.RLock()
	lang := a.language
	catalog := a.messages
	a.languageMu.RUnlock()
	return messageFromCatalog(catalog, lang, status)
}

func messageFromCatalog(catalog map[string]map[int]string, lang string, status int) string {
	if status >= 500 {
		status = http.StatusInternalServerError
	}
	lang = normalizeLanguage(lang)
	if catalog == nil {
		catalog = defaultMessages()
	}
	if messages, ok := catalog[lang]; ok {
		if message, ok := messages[status]; ok && strings.TrimSpace(message) != "" {
			return message
		}
	}
	if messages, ok := catalog[defaultLanguage]; ok {
		if message, ok := messages[status]; ok && strings.TrimSpace(message) != "" {
			return message
		}
	}
	if status >= 500 {
		return http.StatusText(http.StatusInternalServerError)
	}
	return http.StatusText(status)
}
