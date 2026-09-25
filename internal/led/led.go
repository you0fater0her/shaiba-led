package led

import "time"

// RGB — цвет пикселя.
type RGB struct{ R, G, B uint8 }

var Black = RGB{0, 0, 0}

// Strip — аппаратный бэкенд ленты. Реализации: ws281x (build tag ws281x) и NullStrip.
type Strip interface {
	Count() int
	SetPixel(i int, c RGB)
	Render() error // отправить буфер в ленту
	Close() error
}

// NullStrip — заглушка (сборка без тега ws281x, отладка на ПК).
/*type NullStrip struct{ n int }

func NewNullStrip(n int) *NullStrip { return &NullStrip{n: n} }

func (s *NullStrip) Count() int        { return s.n }
func (s *NullStrip) SetPixel(i int, c RGB) {}
func (s *NullStrip) Render() error     { return nil }
func (s *NullStrip) Close() error      { return nil }*/

// RainbowColor — цвет радуги для пикселя i из count, с фазой offset (0..255).
// Та же формула wheel(), что была в Python.
func RainbowColor(i, count, offset int) RGB {
	return Wheel(i*256/count + offset)
}

// Wheel — классическая Adafruit color wheel.
func Wheel(pos int) RGB {
	pos &= 255
	switch {
	case pos < 85:
		return RGB{uint8(pos * 3), uint8(255 - pos*3), 0}
	case pos < 170:
		pos -= 85
		return RGB{uint8(255 - pos*3), 0, uint8(pos * 3)}
	default:
		pos -= 170
		return RGB{0, uint8(pos * 3), uint8(255 - pos*3)}
	}
}

// rainbowOffset — время-зависимая фаза (~50 шагов/сек, как в Python).
func rainbowOffset() int {
	return int(time.Now().UnixMilli()/20) % 256
}