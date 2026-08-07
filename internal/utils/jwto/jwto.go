package jwto

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vertesan/solis-server/internal/utils/config"
	"go.uber.org/zap"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"

func ParseRS256Token(raw string) (*jwt.Token, *jwt.MapClaims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}))
	var claims jwt.MapClaims
	token, err := parser.ParseWithClaims(raw, &claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != "RS256" {
			return nil, fmt.Errorf("invalid sign method: %s", token.Method.Alg())
		}
		keyData, err := config.GetRSAPublicKey()
		if err != nil {
			return nil, err
		}
		return jwt.ParseRSAPublicKeyFromPEM(keyData)
	})

	if err != nil {
		return nil, nil, err
	}
	if !token.Valid {
		zap.L().Error("invalid JWT signature")
		return nil, nil, errors.New("invalid JWT signature")
	}

	exp, err := claims.GetExpirationTime()
	if err != nil {
		return nil, nil, err
	}
	if exp.Before(time.Now()) {
		zap.L().Error("token expired")
		return nil, nil, errors.New("token expired")
	}

	return token, &claims, nil
}

func ParseHS256Token(raw string) (*jwt.Token, *jwt.MapClaims, error) {
	token, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		return config.GetHMACKey()
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, nil, err
	}

	if !token.Valid {
		zap.L().Error("invalid JWT signature")
		return nil, nil, errors.New("invalid JWT signature")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, nil, err
	}

	exp, err := claims.GetExpirationTime()
	if err != nil {
		return nil, nil, err
	}
	if exp.Before(time.Now()) {
		zap.L().Error("token expired")
		return nil, nil, errors.New("token expired")
	}

	return token, &claims, nil
}

func CreateToken(token *jwt.Token) (string, error) {
	keyData, err := config.GetRSAPrivateKey()
	if err != nil {
		return "", err
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(keyData)
	if err != nil {
		return "", err
	}
	raw, err := token.SignedString(key)
	if err != nil {
		return "", err
	}

	return raw, nil
}

func CreateGameAuthToken(token *jwt.Token) (string, error) {
	key, err := config.GetHMACKey()
	if err != nil {
		return "", err
	}
	tokenString, err := token.SignedString(key)
	if err != nil {
		zap.L().Error("failed creating game auth token", zap.Error(err))
		return "", err
	}
	return tokenString, nil
}

func CreateInstallationRefreshToken() string {
	prefix := []byte("3_AS3qfw")
	length := 105
	result := make([]byte, length+len(prefix))
	copy(result, prefix)
	for i := len(prefix); i < len(result); i++ {
		result[i] = charset[rand.Intn(63)]
	}
	return string(result)
}

func CreateAndroidApiToken() string {
	prefix := []byte("APA91b")
	length := 113
	result := make([]byte, length+len(prefix))
	copy(result, prefix)
	for i := len(prefix); i < len(result); i++ {
		result[i] = charset[rand.Intn(63)]
	}
	return string(result)
}

func CreateSecureRefreshToken() string {
	prefix := []byte("AMf-v")
	length := 200
	result := make([]byte, length+len(prefix))
	copy(result, prefix)
	for i := len(prefix); i < len(result); i++ {
		result[i] = charset[rand.Intn(63)]
	}
	return string(result)
}
