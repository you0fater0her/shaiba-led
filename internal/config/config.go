package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Telegram struct {
	Token   string
	AdminID int64
	Proxy   string // socks5://, http://, https:// или пусто
}

type Weather struct {
	APIKey       string
	Lat, Lon     string
	CacheMinutes int
}

type LED struct {
	GPIO       int
	Count      int
	Brightness int
}

type Config struct {
	Telegram Telegram
	Weather  Weather
	LED      LED
	NewYear  time.Time
	DataDir  string
}

// Load читает .env (без перекрытия реальных env) и переменные окружения.
// Имена переменных совместимы с Python-версией.
func Load() (*Config, error) {
	loadDotEnv(".env")

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is not set")
	}
	adminID, _ := strconv.ParseInt(os.Getenv("TELEGRAM_ADMIN_ID"), 10, 64)

	gpio := 18
	if s := os.Getenv("LED_PIN"); s != "" {
		if _, err := fmt.Sscanf(strings.ToUpper(s), "D%d", &gpio); err != nil {
			gpio, _ = strconv.Atoi(s)
		}
	}
	count, _ := strconv.Atoi(getEnv("LED_COUNT", "392"))
	brightness, _ := strconv.Atoi(getEnv("LED_BRIGHTNESS", "255"))

	cacheMin, _ := strconv.Atoi(getEnv("WEATHER_CACHE_MINUTES", "30"))

	ny, err := time.ParseInLocation("2006-01-02 15:04:05",
		getEnv("NEW_YEAR_DATE", "2026-01-01 00:00:00"), time.Local)
	if err != nil {
		return nil, fmt.Errorf("NEW_YEAR_DATE: %w", err)
	}

	dataDir := getEnv("DATA_DIR", "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}

	return &Config{
		Telegram: Telegram{Token: token, AdminID: adminID, Proxy: os.Getenv("TELEGRAM_PROXY_URL")},
		Weather: Weather{
			APIKey: os.Getenv("YANDEX_WEATHER_API_KEY"),
			Lat:    getEnv("YANDEX_WEATHER_LAT", "59.873546"),
			Lon:    getEnv("YANDEX_WEATHER_LON", "29.827624"),
			CacheMinutes: cacheMin,
		},
		LED:     LED{GPIO: gpio, Count: count, Brightness: brightness},
		NewYear: ny,
		DataDir: filepath.Clean(dataDir),
	}, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Мини-парсер .env: KEY=VALUE, # комментарии, кавычки снимаются.
// Реальные переменные окружения имеют приоритет.
func loadDotEnv(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}