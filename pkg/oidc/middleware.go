package oidc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// Validator validates OIDC tokens.
type Validator struct {
	config *Config
	logger *zap.Logger
	jwks   jwkset.Storage
}

// NewValidator creates a new OIDC validator.
func NewValidator(config *Config, logger *zap.Logger) (validator *Validator) {
	// Create JWKS client to fetch public keys from OIDC provider
	jwksURL := fmt.Sprintf("%s/.well-known/jwks.json", strings.TrimSuffix(config.IssuerURL, "/"))

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create JWKS storage with auto-refresh
	options := jwkset.HTTPClientStorageOptions{
		Client:          httpClient,
		RefreshInterval: time.Hour,
		RefreshErrorHandler: func(ctx context.Context, err error) {
			logger.Error("failed to refresh JWKS", zap.Error(err))
		},
	}

	storage, err := jwkset.NewStorageFromHTTP(jwksURL, options)
	if err != nil {
		logger.Error("failed to create JWKS storage", zap.Error(err), zap.String("url", jwksURL))
		// Fall back to memory storage
		storage = jwkset.NewMemoryStorage()
	}

	validator = &Validator{
		config: config,
		logger: logger,
		jwks:   storage,
	}

	return validator
}

// ValidateToken validates an OIDC token.
func (v *Validator) ValidateToken(tokenString string) (claims jwt.MapClaims, err error) {
	// Parse token
	var token *jwt.Token
	token, err = jwt.Parse(tokenString, v.getKeyFunc)

	if err != nil {
		err = fmt.Errorf("failed to parse token: %w", err)
		return claims, err
	}

	// Validate token claims
	var mapClaims jwt.MapClaims
	var claimsOK bool
	mapClaims, claimsOK = token.Claims.(jwt.MapClaims)
	if !claimsOK || !token.Valid {
		err = errors.New("invalid token claims")
		return claims, err
	}

	// Verify issuer
	err = v.verifyIssuer(mapClaims)
	if err != nil {
		return claims, err
	}

	// Verify audience
	err = v.verifyAudience(mapClaims)
	if err != nil {
		return claims, err
	}

	// Verify expiration
	err = v.verifyExpiration(mapClaims)
	if err != nil {
		return claims, err
	}

	// Verify group membership
	err = v.verifyGroupMembership(mapClaims)
	if err != nil {
		return claims, err
	}

	claims = mapClaims
	return claims, err
}

// verifyIssuer verifies the token issuer claim.
func (v *Validator) verifyIssuer(mapClaims jwt.MapClaims) (err error) {
	issuer, ok := mapClaims["iss"].(string)
	if !ok || issuer != v.config.IssuerURL {
		err = fmt.Errorf("invalid issuer: expected %s, got %s", v.config.IssuerURL, issuer)
		return err
	}

	return err
}

// verifyAudience verifies the token audience claim.
func (v *Validator) verifyAudience(mapClaims jwt.MapClaims) (err error) {
	var audiences []string
	audiences, err = v.extractAudiences(mapClaims)
	if err != nil {
		return err
	}

	validAudience := false
	for _, aud := range audiences {
		if aud == v.config.Audience {
			validAudience = true
			break
		}
	}
	if !validAudience {
		err = fmt.Errorf("invalid audience: expected %s, got %v", v.config.Audience, audiences)
		return err
	}

	return err
}

// extractAudiences extracts audience values from token claims.
func (v *Validator) extractAudiences(mapClaims jwt.MapClaims) (audiences []string, err error) {
	switch aud := mapClaims["aud"].(type) {
	case string:
		audiences = []string{aud}
	case []interface{}:
		for _, a := range aud {
			audStr, strOK := a.(string)
			if strOK {
				audiences = append(audiences, audStr)
			}
		}
	default:
		err = errors.New("invalid audience claim type")
		return audiences, err
	}

	return audiences, err
}

// verifyExpiration verifies the token expiration claim.
func (v *Validator) verifyExpiration(mapClaims jwt.MapClaims) (err error) {
	exp, ok := mapClaims["exp"].(float64)
	if !ok {
		err = errors.New("missing or invalid exp claim")
		return err
	}
	if time.Now().Unix() > int64(exp) {
		err = errors.New("token expired")
		return err
	}

	return err
}

// verifyGroupMembership verifies the user is in allowed groups.
func (v *Validator) verifyGroupMembership(mapClaims jwt.MapClaims) (err error) {
	if len(v.config.AllowedGroups) == 0 {
		return err
	}

	var userGroups []string
	userGroups, err = v.extractUserGroups(mapClaims)
	if err != nil {
		return err
	}

	hasValidGroup := false
	for _, userGroup := range userGroups {
		for _, allowedGroup := range v.config.AllowedGroups {
			if userGroup == allowedGroup {
				hasValidGroup = true
				break
			}
		}
		if hasValidGroup {
			break
		}
	}

	if !hasValidGroup {
		err = fmt.Errorf("user not in allowed groups. User groups: %v, allowed: %v",
			userGroups, v.config.AllowedGroups)
		return err
	}

	return err
}

// extractUserGroups extracts user groups from token claims.
func (v *Validator) extractUserGroups(mapClaims jwt.MapClaims) (userGroups []string, err error) {
	groupsInterface, groupsOK := mapClaims["groups"]
	if !groupsOK {
		err = errors.New("token missing groups claim")
		return userGroups, err
	}

	switch groups := groupsInterface.(type) {
	case []interface{}:
		for _, g := range groups {
			groupStr, strOK := g.(string)
			if strOK {
				userGroups = append(userGroups, groupStr)
			}
		}
	case []string:
		userGroups = groups
	default:
		err = errors.New("invalid groups claim type")
		return userGroups, err
	}

	return userGroups, err
}

// getKeyFunc returns a JWT key function for token verification.
func (v *Validator) getKeyFunc(token *jwt.Token) (key interface{}, err error) {
	// Verify signing method
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		err = fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		return key, err
	}

	// Get key ID from token header
	var kid string
	var kidOK bool
	kid, kidOK = token.Header["kid"].(string)
	if !kidOK {
		err = errors.New("token missing kid header")
		return key, err
	}

	// Look up key in JWKS
	var jwk jwkset.JWK
	jwk, err = v.jwks.KeyRead(context.Background(), kid)
	if err != nil {
		err = fmt.Errorf("failed to find key %s in JWKS: %w", kid, err)
		return key, err
	}

	// Extract public key from the JWK
	publicKey := jwk.Key()
	if publicKey == nil {
		err = fmt.Errorf("key %s has no public key", kid)
		return key, err
	}

	key = publicKey
	return key, err
}

// Middleware returns a Gin middleware function for OIDC authentication.
func Middleware(validator *Validator) (handler gin.HandlerFunc) {
	handler = func(ctx *gin.Context) {
		// Extract Bearer token from Authorization header
		authHeader := ctx.GetHeader("Authorization")
		if authHeader == "" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization header",
			})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid authorization header format",
			})
			return
		}

		tokenString := parts[1]

		// Validate token
		claims, err := validator.ValidateToken(tokenString)
		if err != nil {
			validator.logger.Warn("token validation failed",
				zap.Error(err),
				zap.String("remote_addr", ctx.ClientIP()),
			)
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": fmt.Sprintf("invalid token: %v", err),
			})
			return
		}

		// Store claims in context for downstream handlers
		ctx.Set("oidc_claims", claims)

		// Extract and store common claims
		if email, ok := claims["email"].(string); ok {
			ctx.Set("user_email", email)
		}
		if sub, ok := claims["sub"].(string); ok {
			ctx.Set("user_id", sub)
		}

		validator.logger.Debug("request authenticated",
			zap.String("user_email", ctx.GetString("user_email")),
			zap.String("user_id", ctx.GetString("user_id")),
			zap.String("path", ctx.Request.URL.Path),
		)

		ctx.Next()
	}

	return handler
}
