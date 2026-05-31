package sgin

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// UserAccount 是框架内置用户账号记录。
// 它实现 AdminUser，可直接参与 IsAdmin 权限判断。
type UserAccount struct {
	ID                    uint          `json:"id" gorm:"primaryKey"`
	Username              string        `json:"username" gorm:"uniqueIndex;size:191;not null"`
	PasswordHash          string        `json:"-" gorm:"not null"`
	Admin                 bool          `json:"is_admin" gorm:"not null;default:false"`
	Enabled               bool          `json:"enabled" gorm:"not null;default:true"`
	Groups                []AccessGroup `json:"groups,omitempty" gorm:"many2many:sgin_user_groups;"`
	Roles                 []AccessRole  `json:"roles,omitempty" gorm:"many2many:sgin_user_roles;"`
	RefreshTokenHash      string        `json:"-"`
	RefreshTokenExpiresAt *time.Time    `json:"-"`
	CreatedAt             time.Time     `json:"created_at"`
	UpdatedAt             time.Time     `json:"updated_at"`
}

// AdminAccount 保留为兼容别名，实际用户模型是 UserAccount。
type AdminAccount = UserAccount

// TableName 固定内置用户表名。
func (UserAccount) TableName() string {
	return "sgin_users"
}

// IsAdmin 返回该账号是否具备管理员权限。
func (u UserAccount) IsAdmin() bool {
	if u.Admin {
		return true
	}
	for _, group := range u.Groups {
		if strings.EqualFold(group.Name, "admin") {
			return true
		}
	}
	return false
}

// UserBootstrapResult 描述启动时用户系统初始化结果。
type UserBootstrapResult struct {
	Enabled       bool
	AdminCreated  bool
	AdminUsername string
	AdminPassword string
}

// AdminBootstrapResult 保留旧版管理员初始化结果字段。
type AdminBootstrapResult struct {
	Enabled  bool
	Created  bool
	Username string
	Password string
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type tokenResponse struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	TokenType        string `json:"token_type"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

type authClaims struct {
	Username  string `json:"username"`
	Admin     bool   `json:"admin"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

type authTokenError string

func (e authTokenError) Error() string {
	return string(e)
}

func authTokenErrorCode(err error) string {
	var tokenErr authTokenError
	if errors.As(err, &tokenErr) {
		return string(tokenErr)
	}
	return ErrCodeInvalidToken
}

func bootstrapUserSystem(cfg Config) (UserBootstrapResult, error) {
	result := UserBootstrapResult{
		Enabled:       cfg.User.Enabled,
		AdminUsername: cfg.User.Admin.Username,
	}
	if !cfg.User.Enabled {
		return result, nil
	}

	db, err := openUserDB(cfg)
	if err != nil {
		return result, err
	}
	defer closeGormDB(db)

	if err := migrateAccessModels(db); err != nil {
		return result, err
	}
	if !cfg.User.Admin.Init {
		return result, nil
	}

	var account UserAccount
	err = db.Where("username = ?", cfg.User.Admin.Username).First(&account).Error
	if err == nil {
		if err := ensureBuiltinAdminAccess(db, account); err != nil {
			return result, err
		}
		return result, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return result, err
	}

	password, err := generateAdminPassword()
	if err != nil {
		return result, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return result, err
	}

	account = UserAccount{
		Username:     cfg.User.Admin.Username,
		PasswordHash: string(hash),
		Admin:        true,
		Enabled:      true,
	}
	if err := db.Create(&account).Error; err != nil {
		var existing UserAccount
		if checkErr := db.Where("username = ?", cfg.User.Admin.Username).First(&existing).Error; checkErr == nil {
			if err := ensureBuiltinAdminAccess(db, existing); err != nil {
				return result, err
			}
			return result, nil
		}
		return result, err
	}
	if err := ensureBuiltinAdminAccess(db, account); err != nil {
		return result, err
	}

	result.AdminCreated = true
	result.AdminPassword = password
	return result, nil
}
func openUserDB(cfg Config) (*gorm.DB, error) {
	return openGormDB(cfg)
}

func openGormDB(cfg Config) (*gorm.DB, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
	dsn := cfg.Database.DSN
	if dsn == "" {
		dsn = DefaultConfig().Database.DSN
	}

	switch driver {
	case "", "sqlite", "sqlite3":
		if err := ensureSQLiteParentDir(dsn); err != nil {
			return nil, err
		}
		return gorm.Open(sqlite.Open(dsn), userGormConfig())
	case "mysql":
		return gorm.Open(mysql.Open(dsn), userGormConfig())
	case "pg", "postgres", "postgresql":
		return gorm.Open(postgres.Open(dsn), userGormConfig())
	default:
		return nil, fmt.Errorf("sgin: unsupported user database driver %q", cfg.Database.Driver)
	}
}

func userGormConfig() *gorm.Config {
	return &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	}
}

func closeGormDB(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func registerUserRoutes(app *App) {
	if app == nil || !app.config.User.Enabled {
		return
	}
	loginPath := normalizeUserPath(app.config.User.Path)
	refreshPath := strings.TrimRight(loginPath, "/") + "/refresh"
	if loginPath == "/" {
		refreshPath = "/refresh"
	}
	app.POST(loginPath, app.handleLogin)
	app.POST(refreshPath, app.handleRefreshToken)
}

func normalizeUserPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/login"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "/"
	}
	return path
}

func (a *App) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSON(c, http.StatusBadRequest, Fail(http.StatusBadRequest, "invalid login request", err.Error()))
		return
	}

	db, err := openUserDB(a.config)
	if err != nil {
		HandleError(c, err)
		return
	}
	defer closeGormDB(db)

	var user UserAccount
	err = db.Where("username = ?", req.Username).First(&user).Error
	if err != nil {
		JSON(c, http.StatusUnauthorized, Fail(http.StatusUnauthorized, "invalid username or password", ErrCodeInvalidCredentials))
		return
	}
	if !user.Enabled {
		JSON(c, http.StatusUnauthorized, Fail(http.StatusUnauthorized, "account disabled", ErrCodeAccountDisabled))
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		JSON(c, http.StatusUnauthorized, Fail(http.StatusUnauthorized, "invalid username or password", ErrCodeInvalidCredentials))
		return
	}

	resp, err := a.issueTokenPair(db, &user)
	if err != nil {
		HandleError(c, err)
		return
	}
	JSON(c, http.StatusOK, OK(resp))
}

func (a *App) handleRefreshToken(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		JSON(c, http.StatusBadRequest, Fail(http.StatusBadRequest, "invalid refresh request", err.Error()))
		return
	}

	claims, err := parseAuthToken(a.config, req.RefreshToken, "refresh")
	if err != nil {
		code := ErrCodeInvalidRefreshToken
		message := "invalid refresh token"
		if authTokenErrorCode(err) == ErrCodeTokenExpired {
			code = ErrCodeRefreshTokenExpired
			message = "refresh token expired"
		}
		JSON(c, http.StatusUnauthorized, Fail(http.StatusUnauthorized, message, code))
		return
	}
	userID, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil {
		JSON(c, http.StatusUnauthorized, Fail(http.StatusUnauthorized, "invalid refresh token", ErrCodeInvalidRefreshToken))
		return
	}

	db, err := openUserDB(a.config)
	if err != nil {
		HandleError(c, err)
		return
	}
	defer closeGormDB(db)

	var user UserAccount
	if err := db.First(&user, uint(userID)).Error; err != nil {
		JSON(c, http.StatusUnauthorized, Fail(http.StatusUnauthorized, "invalid refresh token", ErrCodeInvalidRefreshToken))
		return
	}
	if user.RefreshTokenHash == "" || user.RefreshTokenHash != hashToken(req.RefreshToken) {
		JSON(c, http.StatusUnauthorized, Fail(http.StatusUnauthorized, "invalid refresh token", ErrCodeInvalidRefreshToken))
		return
	}
	if user.RefreshTokenExpiresAt == nil || time.Now().After(*user.RefreshTokenExpiresAt) {
		JSON(c, http.StatusUnauthorized, Fail(http.StatusUnauthorized, "refresh token expired", ErrCodeRefreshTokenExpired))
		return
	}

	resp, err := a.issueTokenPair(db, &user)
	if err != nil {
		HandleError(c, err)
		return
	}
	JSON(c, http.StatusOK, OK(resp))
}

func (a *App) issueTokenPair(db *gorm.DB, user *UserAccount) (tokenResponse, error) {
	accessTTL := time.Duration(a.config.JWT.Expired) * time.Hour
	refreshTTL := time.Duration(a.config.JWT.RefreshExpired) * time.Hour
	accessToken, err := makeAuthToken(a.config, user, "access", accessTTL)
	if err != nil {
		return tokenResponse{}, err
	}
	refreshToken, err := makeAuthToken(a.config, user, "refresh", refreshTTL)
	if err != nil {
		return tokenResponse{}, err
	}
	expiresAt := time.Now().Add(refreshTTL)
	user.RefreshTokenHash = hashToken(refreshToken)
	user.RefreshTokenExpiresAt = &expiresAt
	if err := db.Save(user).Error; err != nil {
		return tokenResponse{}, err
	}
	return tokenResponse{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		TokenType:        "Bearer",
		ExpiresIn:        int(accessTTL.Seconds()),
		RefreshExpiresIn: int(refreshTTL.Seconds()),
	}, nil
}

func makeAuthToken(cfg Config, user *UserAccount, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now()
	jti, err := generateRandomSecret(16)
	if err != nil {
		return "", err
	}
	claims := authClaims{
		Username:  user.Username,
		Admin:     user.Admin,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatUint(uint64(user.ID), 10),
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.JWT.Secret))
}

func parseAuthToken(cfg Config, tokenText string, tokenType string) (*authClaims, error) {
	claims := &authClaims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("sgin: unexpected jwt signing method")
		}
		return []byte(cfg.JWT.Secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, authTokenError(ErrCodeTokenExpired)
		}
		return nil, authTokenError(ErrCodeInvalidToken)
	}
	if token == nil || !token.Valid {
		return nil, authTokenError(ErrCodeInvalidToken)
	}
	if claims.TokenType != tokenType {
		return nil, authTokenError(ErrCodeInvalidTokenType)
	}
	return claims, nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// JWTAuth 返回基于 access token 的认证中间件。
// 验证成功后会把 UserAccount 写入 Gin Context 的 user。
func (a *App) JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		tokenText, ok := bearerToken(header)
		if strings.TrimSpace(header) == "" {
			writeDecisionDenied(c, Deny(ErrCodeAuthenticationRequired, "authentication required"))
			c.Abort()
			return
		}
		if !ok {
			writeDecisionDenied(c, Deny(ErrCodeInvalidAuthorization, "invalid authorization header"))
			c.Abort()
			return
		}
		claims, err := parseAuthToken(a.config, tokenText, "access")
		if err != nil {
			code := authTokenErrorCode(err)
			message := "invalid access token"
			if code == ErrCodeTokenExpired {
				message = "access token expired"
			}
			writeDecisionDenied(c, Deny(code, message))
			c.Abort()
			return
		}
		userID, err := strconv.ParseUint(claims.Subject, 10, 64)
		if err != nil {
			writeDecisionDenied(c, Deny(ErrCodeInvalidToken, "invalid access token"))
			c.Abort()
			return
		}

		db, err := openUserDB(a.config)
		if err != nil {
			HandleError(c, err)
			c.Abort()
			return
		}
		defer closeGormDB(db)

		var user UserAccount
		if err := db.First(&user, uint(userID)).Error; err != nil {
			writeDecisionDenied(c, Deny(ErrCodeInvalidToken, "invalid access token"))
			c.Abort()
			return
		}
		if !user.Enabled {
			writeDecisionDenied(c, Deny(ErrCodeAccountDisabled, "account disabled"))
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], true
}

func generateAdminPassword() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func ensureSQLiteParentDir(dsn string) error {
	if dsn == "" || dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return nil
	}
	dir := filepath.Dir(dsn)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
