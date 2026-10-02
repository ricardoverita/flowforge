package platform

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL        string
	NATSURL            string
	NATSToken          string
	APIToken           string
	HTTPAddr           string
	ServiceName        string
	OTLPEndpoint       string
	PollInterval       time.Duration
	WorkerDelay        time.Duration
	WorkerFailTask     string
	WorkerFailAttempts int
}

func Load(service string) (Config, error) {
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), NATSURL: env("NATS_URL", "nats://127.0.0.1:4222"), APIToken: os.Getenv("API_TOKEN"), ServiceName: env("SERVICE_NAME", "flowforge-"+service), OTLPEndpoint: os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), WorkerFailTask: os.Getenv("WORKER_FAIL_TASK")}
	port := map[string]string{"api": "8080", "engine": "8081", "worker": "8082", "migrate": "8083"}[service]
	c.HTTPAddr = env("HTTP_ADDR", ":"+port)
	_, portNumber, addressErr := net.SplitHostPort(c.HTTPAddr)
	parsedPort, parseErr := strconv.Atoi(portNumber)
	if addressErr != nil || parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
		return c, fmt.Errorf("HTTP_ADDR must contain host and a port between 1 and 65535")
	}
	var err error
	if service == "engine" || service == "worker" {
		c.NATSToken, err = secret("NATS_TOKEN")
		if err != nil {
			return c, err
		}
	}
	if c.DatabaseURL == "" && service != "worker" {
		password, e := secret("DATABASE_PASSWORD")
		if e != nil {
			return c, e
		}
		if password == "" {
			return c, fmt.Errorf("DATABASE_URL or DATABASE_PASSWORD[_FILE] is required")
		}
		u := url.URL{Scheme: "postgres", User: url.UserPassword(env("DATABASE_USER", "flowforge"), password), Host: net.JoinHostPort(env("DATABASE_HOST", "127.0.0.1"), env("DATABASE_PORT", "5432")), Path: "/" + env("DATABASE_NAME", "flowforge")}
		q := u.Query()
		q.Set("sslmode", env("DATABASE_SSLMODE", "require"))
		u.RawQuery = q.Encode()
		c.DatabaseURL = u.String()
	}
	if c.DatabaseURL != "" {
		if u, e := url.Parse(c.DatabaseURL); e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
			return c, fmt.Errorf("DATABASE_URL must be a PostgreSQL URL")
		}
	}
	if u, e := url.Parse(c.NATSURL); e != nil || (u.Scheme != "nats" && u.Scheme != "tls") || u.Host == "" {
		return c, fmt.Errorf("NATS_URL must use nats:// or tls://")
	}
	if c.OTLPEndpoint != "" {
		if u, e := url.Parse(c.OTLPEndpoint); e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return c, fmt.Errorf("OTEL_EXPORTER_OTLP_ENDPOINT must be an HTTP URL")
		}
	}
	c.PollInterval, err = duration("POLL_INTERVAL", "250ms", 10*time.Millisecond, 30*time.Second)
	if err != nil {
		return c, err
	}
	c.WorkerDelay, err = duration("WORKER_DELAY", "200ms", 0, 20*time.Second)
	if err != nil {
		return c, err
	}
	c.WorkerFailAttempts, err = strconv.Atoi(env("WORKER_FAIL_ATTEMPTS", "0"))
	if err != nil || c.WorkerFailAttempts < 0 || c.WorkerFailAttempts > 10 {
		return c, fmt.Errorf("WORKER_FAIL_ATTEMPTS must be between 0 and 10")
	}
	return c, nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func secret(key string) (string, error) {
	value := os.Getenv(key)
	path := os.Getenv(key + "_FILE")
	if value != "" && path != "" {
		return "", fmt.Errorf("set only one of %s and %s_FILE", key, key)
	}
	if path == "" {
		return value, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", key, err)
	}
	value = strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("%s_FILE is empty", key)
	}
	return value, nil
}
func duration(key, fallback string, min, max time.Duration) (time.Duration, error) {
	d, err := time.ParseDuration(env(key, fallback))
	if err != nil || d < min || d > max {
		return 0, fmt.Errorf("%s must be a duration between %s and %s", key, min, max)
	}
	return d, nil
}
