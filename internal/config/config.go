package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort                string
	DatabaseURL            string
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	CognitoRegion          string
	CognitoUserPoolID      string
	CognitoAppClientID     string
	CognitoAppClientSecret string
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		fmt.Println("File .env tidak ditemukan; menggunakan environment variables")
	}

	redisDB, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
	if err != nil {
		return nil, fmt.Errorf("REDIS_DB harus berupa angka: %w", err)
	}

	cfg := &Config{
		AppPort:                getEnv("APP_PORT", "8080"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		RedisAddr:              getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:          os.Getenv("REDIS_PASSWORD"),
		RedisDB:                redisDB,
		CognitoRegion:          getEnv("AWS_REGION", ""),
		CognitoUserPoolID:      getEnv("COGNITO_USER_POOL_ID", ""),
		CognitoAppClientID:     getEnv("COGNITO_CLIENT_ID", ""),
		CognitoAppClientSecret: os.Getenv("COGNITO_CLIENT_SECRET"),
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL belum diatur")
	}

	if cfg.CognitoRegion == "" {
		return nil, fmt.Errorf("AWS_REGION belum diatur")
	}

	if cfg.CognitoUserPoolID == "" {
		return nil, fmt.Errorf("COGNITO_USER_POOL_ID belum diatur")
	}

	if cfg.CognitoAppClientID == "" {
		return nil, fmt.Errorf("COGNITO_CLIENT_ID belum diatur")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value != "" {
		return value
	}

	return fallback
}
