//go:build ws281x

package led

import (
	"fmt"
	"os"

	ws2811 "github.com/rpi-ws281x/rpi-ws281x-go"
)

// WS281xStrip — биндинг к rpi_ws281x (PWM/DMA, требуется cgo).
type WS281xStrip struct {
	dev  *ws2811.WS2811
	leds []uint32
}

func NewWS281xStrip(gpio, count, brightness int) (*WS281xStrip, error) {
	opt := ws2811.DefaultOptions
	opt.Channels[0].GpioPin = gpio
	opt.Channels[0].LedCount = count
	opt.Channels[0].Brightness = brightness
	opt.Channels[0].StripType = ws2811.WS2811StripGRB

	dev, err := ws2811.MakeWS2811(&opt)
	if err != nil {
		return nil, fmt.Errorf("ws2811 init: %w", err)
	}
	if err := dev.Init(); err != nil {
		return nil, fmt.Errorf("ws2811 hardware init: %w", err)
	}
	return &WS281xStrip{dev: dev, leds: dev.Leds(0)}, nil
}

func (s *WS281xStrip) Count() int { return len(s.leds) }

func (s *WS281xStrip) SetPixel(i int, c RGB) {
	if i < 0 || i >= len(s.leds) {
		return
	}
	s.leds[i] = uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}

// Render + Wait: кадр полностью ушёл в ленту, без разрывов.
func (s *WS281xStrip) Render() error {
	if err := s.dev.Render(); err != nil {
		return err
	}
	return s.dev.Wait()
}

func (s *WS281xStrip) Close() error { return s.dev.Fini() }

// OpenStrip: LED_SIM=1 принудительно включает симуляцию даже в pi-сборке.
func OpenStrip(gpio, count, brightness int) (Strip, error) {
	if os.Getenv("LED_SIM") == "1" {
		fmt.Println("LED_SIM=1: using null strip")
		return NewNullStrip(count), nil
	}
	return NewWS281xStrip(gpio, count, brightness)
}