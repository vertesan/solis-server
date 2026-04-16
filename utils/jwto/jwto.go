package jwto

import (
	"errors"
	"fmt"
	"math/rand"
	"time"
	"vertesan/solis-server/utils/config"
	"vertesan/solis-server/utils/rich"

	"github.com/golang-jwt/jwt/v5"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"

func ParseRS256Token(raw string) (*jwt.Token, *jwt.MapClaims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}))
	var claims jwt.MapClaims
	token, err := parser.ParseWithClaims(raw, &claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != "RS256" {
			return nil, fmt.Errorf("invalid sign method: %s", token.Method.Alg())
		}
		return jwt.ParseRSAPublicKeyFromPEM(config.GetRSAPublicKey())
	})

	if err != nil {
		return nil, nil, err
	}
	if !token.Valid {
		rich.Error("invalid JWT signature")
		return nil, nil, errors.New("invalid JWT signature")
	}

	exp, err := claims.GetExpirationTime()
	if err != nil {
		return nil, nil, err
	}
	if exp.Before(time.Now()) {
		rich.Error("token expired")
		return nil, nil, errors.New("token expired")
	}

	return token, &claims, nil
}

func ParseHS256Token(raw string) (*jwt.Token, *jwt.MapClaims, error) {
	token, err := jwt.Parse(raw, func(token *jwt.Token) (interface{}, error) {
		return config.GetHMACKey(), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, nil, err
	}

	if !token.Valid {
		rich.Error("invalid JWT signature")
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
		rich.Error("token expired")
		return nil, nil, errors.New("token expired")
	}

	return token, &claims, nil
}

func CreateToken(token *jwt.Token) (string, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM(config.GetRSAPrivateKey())
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
	tokenString, err := token.SignedString(config.GetHMACKey())
	if err != nil {
		rich.Error("failed creating game auth token: %v", err)
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
