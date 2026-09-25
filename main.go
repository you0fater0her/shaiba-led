package main

import (
	"context"
	"log"
	"os/signal"
	"path/filepath"
	"syscall"

	"ledstrip/internal/botapi"
	"ledstrip/internal/config"
	"ledstrip/internal/display"
	"ledstrip/internal/led"
	"ledstrip/internal/services"
	"ledstrip/internal/storage"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	strip, err := led.OpenStrip(cfg.LED.GPIO, cfg.LED.Count, cfg.LED.Brightness)
	if err != nil {
		log.Fatalf("led strip: %v", err)
	}

	store := storage.New(cfg.DataDir, cfg.Telegram.AdminID)

	ctl := led.NewController(strip)
	ctl.SetColor(store.LoadColor()) // восстанавливаем сохранённый цвет

	weather := services.New(cfg.Weather, filepath.Join(cfg.DataDir, "weather.txt"))
	engine := display.NewEngine(ctl, weather, store, cfg.NewYear)

	bot, err := botapi.New(cfg, engine, store)
	if err != nil {
		log.Fatalf("telegram: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go weather.Run(ctx) // фоновое обновление погоды

	go func() { // Telegram: поллинг с retry; фатальная ошибка гасит приложение
		if err := bot.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("telegram bot stopped: %v", err)
			stop()
		}
	}()

	log.Printf("display engine started (gpio=%d leds=%d)", cfg.LED.GPIO, cfg.LED.Count)
	if err := engine.Run(ctx); err != nil {
		log.Printf("engine: %v", err)
	}
	log.Println("shutdown complete")
}