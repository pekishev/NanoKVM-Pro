package mcp

import (
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"NanoKVM-Server/config"
	"NanoKVM-Server/middleware"
	"NanoKVM-Server/service/auth"
)

func CheckAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		conf := config.GetInstance()
		if conf.Authentication == "disable" {
			c.Next()
			return
		}

		if c.ClientIP() == "127.0.0.1" || c.ClientIP() == "::1" || strings.HasPrefix(c.ClientIP(), "127.") {
			c.Next()
			return
		}

		token, err := c.Cookie("nano-kvm-token")
		if err != nil {
			header := c.GetHeader("Authorization")
			if strings.HasPrefix(header, "Bearer ") {
				token = strings.TrimPrefix(header, "Bearer ")
			}
		}
		if token != "" {
			if _, err := middleware.ParseJWT(token); err == nil {
				c.Next()
				return
			}
		}

		if username, password, ok := parseBasic(c.GetHeader("Authorization")); ok {
			if auth.CompareAccount(username, password) {
				c.Next()
				return
			}
			time.Sleep(2 * time.Second)
		}

		c.Header("WWW-Authenticate", `Basic realm="NanoKVM"`)
		c.JSON(http.StatusUnauthorized, "unauthorized")
		c.Abort()
	}
}

func parseBasic(header string) (string, string, bool) {
	const prefix = "Basic "
	if !strings.HasPrefix(header, prefix) {
		return "", "", false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header[len(prefix):]))
	if err != nil {
		return "", "", false
	}
	username, password, ok := strings.Cut(string(raw), ":")
	if !ok || username == "" {
		return "", "", false
	}
	return username, password, true
}
