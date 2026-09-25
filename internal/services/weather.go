package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Weather — Яндекс.Погода с фоновым обновлением и stale-while-revalidate.
// Python-версия ходила за погодой синхронно из дисплей-цикла — здесь
// цикл никогда не ждёт сеть.
type Weather struct {
	apiKey string
	lat    string
	lon    string
	cache  time.Duration
	file   string
	client *http.Client

	mu   sync.RWMutex
	temp float64 // °C
	has  bool
	at   time.Time
}

func New(cfg struct {
	APIKey       string
	Lat, Lon     string
	CacheMinutes int
}, file string) *Weather {
	w := &Weather{
		apiKey: cfg.APIKey,
		lat:    cfg.Lat,
		lon:    cfg.Lon,
		cache:  time.Duration(cfg.CacheMinutes) * time.Minute,
		file:   file,
		client: &http.Client{Timeout: 10 * time.Second},
	}
	if w.cache <= 0 {
		w.cache = 30 * time.Minute
	}
	w.loadCache()
	return w
}

// loadCache читает data/weather.txt: "<temp> <unix_ts>" (формат наш, см. README).
func (w *Weather) loadCache() {
	b, err := os.ReadFile(w.file)
	if err != nil {
		return
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return
	}
	t, err1 := strconv.ParseFloat(f[0], 64)
	ts, err2 := strconv.ParseInt(f[1], 10, 64)
	if err1 != nil || err2 != nil {
		return
	}
	w.temp, w.has, w.at = t, true, time.Unix(ts, 0)
}

func (w *Weather) saveCache() {
	_ = os.MkdirAll(filepath.Dir(w.file), 0o755)
	_ = os.WriteFile(w.file,
		[]byte(fmt.Sprintf("%.1f %d", w.temp, w.at.Unix())), 0o644)
}

// Run — фоновый цикл обновления, до первого успешного fetch раз в 30 секунд,
// далее — раз в cache.
func (w *Weather) Run(ctx context.Context) {
	if w.apiKey == "" {
		log.Println("weather: YANDEX_WEATHER_API_KEY is empty, temperature disabled")
		return
	}
	delay := 30 * time.Second
	for {
		fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := w.fetch(fetchCtx)
		cancel()
		if err != nil {
			log.Printf("weather: fetch failed (serving cache): %v", err)
		} else {
			delay = w.cache
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func (w *Weather) fetch(ctx context.Context) error {
	url := fmt.Sprintf("https://api.weather.yandex.ru/v2/forecast?lat=%s&lon=%s", w.lat, w.lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Yandex-API-Key", w.apiKey)

	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("yandex weather: %s", resp.Status)
	}

	var payload struct {
		Fact struct {
			Temp float64 `json:"temp"`
		} `json:"fact"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return err
	}

	w.mu.Lock()
	w.temp, w.has, w.at = payload.Fact.Temp, true, time.Now()
	w.mu.Unlock()
	w.saveCache()
	return nil
}

// Temp — температура, если кэш свежий (допуск 2×cache при сбоях сети).
func (w *Weather) Temp() (float64, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if !w.has || time.Since(w.at) > 2*w.cache {
		return 0, false
	}
	return w.temp, true
}

// Display — "+12°", "-3°" (для статичного показа на ленте).
func (w *Weather) Display() string {
	t, ok := w.Temp()
	if !ok {
		return ""
	}
	v := int(math.Round(t))
	sign := "+"
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d°", sign, v)
}