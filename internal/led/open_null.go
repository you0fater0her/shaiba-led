//go:build !ws281x

package led

import (
	"fmt"
	"strings"
)

// OpenStrip без тега ws281x — симулятор (удобно тестировать бота без малинки).
func OpenStrip(_, count, _ int) (Strip, error) {
	fmt.Printf("built without 'ws281x' tag: using null strip (%d leds)\n", count)
	return NewNullStrip(count), nil
}

// NullStrip — симулятор дисплея из 4 семисегментных символов.
type NullStrip struct {
	count      int
	pixels     []RGB
	lastRender []RGB // Хранилище предыдущего кадра для сравнения
}

func NewNullStrip(n int) *NullStrip {
	return &NullStrip{
		count:      n,
		pixels:     make([]RGB, n),
		lastRender: make([]RGB, n),
	}
}

func (s *NullStrip) Count() int {
	return s.count
}

func (s *NullStrip) SetPixel(i int, c RGB) {
	if i >= 0 && i < s.count {
		s.pixels[i] = c
	}
}

func (s *NullStrip) Close() error {
	return nil
}

// Render — отрисовывает кадр в консоли только при наличии изменений.
func (s *NullStrip) Render() error {
	// 1. Проверяем, изменился ли кадр по сравнению с прошлым вызовом
	changed := false
	for i := 0; i < s.count; i++ {
		if s.pixels[i] != s.lastRender[i] {
			changed = true
			break
		}
	}

	// Если ничего не изменилось — пропускаем вывод, чтобы не спамить
	if !changed {
		return nil
	}

	// Сохраняем текущее состояние как последнее отрисованное
	copy(s.lastRender, s.pixels)

	// 2. Логика получения цвета сегмента
	getSegColor := func(digitIdx, segNum int) (RGB, bool) {
		start := (digitIdx * 98) + (segNum-1)*14
		end := start + 14
		if end > len(s.pixels) {
			return Black, false
		}

		for i := start; i < end; i++ {
			c := s.pixels[i]
			if c.R > 0 || c.G > 0 || c.B > 0 {
				return c, true
			}
		}
		return Black, false
	}

	formatBlock := func(c RGB, active bool, horizontal bool) string {
		if !active {
			if horizontal {
				return "     "
			}
			return " "
		}
		if horizontal {
			return fmt.Sprintf("\x1b[38;2;%d;%d;%dm█████\x1b[0m", c.R, c.G, c.B)
		}
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm█\x1b[0m", c.R, c.G, c.B)
	}

	var lines [5]strings.Builder

	for d := 0; d < 4; d++ {
		c1, a1 := getSegColor(d, 1) // верх
		c2, a2 := getSegColor(d, 2) // верх-право
		c3, a3 := getSegColor(d, 3) // низ-право
		c4, a4 := getSegColor(d, 4) // низ
		c5, a5 := getSegColor(d, 5) // низ-лево
		c6, a6 := getSegColor(d, 6) // верх-лево
		c7, a7 := getSegColor(d, 7) // середина

		lines[0].WriteString(" " + formatBlock(c1, a1, true) + "   ")
		lines[1].WriteString(formatBlock(c6, a6, false) + "     " + formatBlock(c2, a2, false) + "  ")
		lines[2].WriteString(" " + formatBlock(c7, a7, true) + "   ")
		lines[3].WriteString(formatBlock(c5, a5, false) + "     " + formatBlock(c3, a3, false) + "  ")
		lines[4].WriteString(" " + formatBlock(c4, a4, true) + "   ")
	}

	// 3. Вывод с очисткой: ANSI-код \x1b[H\x1b[2J очищает терминал и возвращает курсор наверх
	fmt.Print("\x1b[H\x1b[2J")
	fmt.Println("┌─────────────────────────────── LED FRAME ───────────────────────────────┐")
	for _, l := range lines {
		fmt.Println("│ " + l.String() + "│")
	}
	fmt.Println("└─────────────────────────────────────────────────────────────────────────┘")

	return nil
}