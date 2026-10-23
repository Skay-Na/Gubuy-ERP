package middlewares

import (
	"erp-backend/config"
	"erp-backend/models"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const adminTokenDuration = 7 * 24 * time.Hour

var JwtKey []byte

var errJWTSecretMissing = errors.New("JWT_SECRET must be configured with at least 32 bytes")

// ConfigureJWTSecret installs the signing key from explicit configuration.
// It intentionally has no built-in fallback so production cannot silently
// issue tokens with a publicly known secret.
func ConfigureJWTSecret(secret string) error {
	if len([]byte(secret)) < 32 {
		JwtKey = nil
		return errJWTSecretMissing
	}
	JwtKey = []byte(secret)
	return nil
}

func LoadJWTSecretFromEnvironment() error {
	return ConfigureJWTSecret(os.Getenv("JWT_SECRET"))
}

type Claims struct {
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// GenerateToken 签发管理员 JWT，登录后七天内无需再次输入密码。
func GenerateToken(username string) (string, error) {
	return GenerateTokenWithDuration(username, adminTokenDuration)
}

// GenerateTokenWithDuration 按指定有效期签发 JWT
func GenerateTokenWithDuration(username string, duration time.Duration) (string, error) {
	if len(JwtKey) < 32 {
		return "", errJWTSecretMissing
	}
	expirationTime := time.Now().Add(duration)
	claims := &Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JwtKey)
}

func authenticate(c *gin.Context) bool {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "未登录，无权访问"})
		c.Abort()
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if !(len(parts) == 2 && parts[0] == "Bearer") {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "Token 格式错误"})
		c.Abort()
		return false
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (interface{}, error) {
		if len(JwtKey) < 32 {
			return nil, errJWTSecretMissing
		}
		return JwtKey, nil
	})
	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "无效的 Token 或 Token 已过期"})
		c.Abort()
		return false
	}

	c.Set("username", claims.Username)
	return true
}

// AuthRequired JWT 认证中间件
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authenticate(c) {
			return
		}
		username := c.GetString("username")
		if !strings.HasPrefix(username, "emp_") {
			return
		}
		var employee models.Employee
		if err := config.DB.Where("emp_no = ?", strings.TrimPrefix(username, "emp_")).First(&employee).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
			c.Abort()
			return
		}
		if !employee.CanUseEmployeeAccess(time.Now()) {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工已离职，终端权限已停用"})
			c.Abort()
		}
	}
}

// AdminAuthRequired 管理员权限校验中间件。员工身份采用 emp_ 前缀，管理员账号不使用该前缀。
func AdminAuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authenticate(c) {
			return
		}
		if strings.HasPrefix(c.GetString("username"), "emp_") {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "需要管理员权限"})
			c.Abort()
		}
	}
}
