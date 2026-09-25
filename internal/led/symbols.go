package led

import (
	"math/rand"
	"strings"
)

// Геометрия: символ = 7 сегментов по 14 LED, 98 LED на символ, 4 позиции.
// Нумерация сегментов: 1=верх, 2=верх-право, 3=низ-право, 4=низ,
// 5=низ-лево, 6=верх-лево, 7=середина. Все сегменты целиком (упрощение
// относительно Python-версии: там частичные диапазоны, визуальный
// результат эквивалентен).
var Font = map[rune][]int{
	'0': {1, 2, 3, 4, 5, 6}, '1': {2, 3}, '2': {1, 2, 7, 5, 4},
	'3': {1, 2, 7, 3, 4}, '4': {6, 7, 2, 3}, '5': {1, 6, 7, 3, 4},
	'6': {1, 6, 7, 5, 3, 4}, '7': {1, 2, 3}, '8': {1, 2, 3, 4, 5, 6, 7},
	'9': {1, 6, 7, 2, 3, 4},
	'A': {1, 2, 3, 5, 6, 7}, 'B': {1, 6, 7, 5, 3, 4}, 'C': {1, 6, 5, 4},
	'D': {2, 3, 4, 5, 6}, 'E': {1, 6, 7, 5, 4}, 'F': {1, 6, 7, 5},
	'G': {1, 6, 5, 4, 3}, 'H': {6, 7, 2, 3, 5}, 'I': {2, 3},
	'J': {2, 3, 4, 5}, 'K': {6, 5, 7, 2}, 'L': {6, 5, 4}, 'M': {1, 2, 3, 5, 6},
	'N': {6, 5, 2, 3}, 'O': {1, 2, 3, 4, 5, 6}, 'P': {1, 2, 6, 7, 5},
	'Q': {1, 2, 3, 5, 6, 7}, 'R': {1, 2, 6, 7, 5, 3}, 'S': {1, 6, 7, 3, 4},
	'T': {1, 2, 3}, 'U': {6, 5, 4, 3, 2}, 'V': {6, 5, 4, 3, 2},
	'W': {6, 5, 4, 3, 2, 7}, 'X': {6, 5, 7, 2, 3}, 'Y': {2, 3, 4, 6, 7},
	'Z': {1, 2, 7, 5, 4},
	// кириллица (аппроксимации под 7 сегментов)
	'А': {1, 2, 3, 5, 6, 7}, 'В': {1, 6, 7, 5, 3, 4}, 'Е': {1, 6, 7, 5, 4},
	'Ё': {1, 6, 7, 5, 4}, 'К': {6, 5, 7, 2}, 'М': {1, 2, 3, 5, 6},
	'Н': {6, 7, 2, 3, 5}, 'О': {1, 2, 3, 4, 5, 6}, 'Р': {1, 2, 6, 7, 5},
	'С': {1, 6, 5, 4}, 'Т': {1, 2, 3}, 'У': {2, 3, 4, 6, 7},
	'Х': {6, 5, 7, 2, 3}, 'З': {1, 2, 7, 3, 4}, 'И': {6, 5, 2, 3},
	'Й': {6, 5, 2, 3}, 'Л': {2, 3, 4, 5, 6}, 'П': {1, 2, 3, 5, 6},
	'Г': {1, 6}, 'Д': {2, 3, 4, 5, 7}, 'Ж': {6, 5, 7, 2, 3},
	'Ч': {6, 7, 3}, 'Ш': {6, 5, 4, 3, 2}, 'Щ': {6, 5, 4, 3, 2},
	'Ъ': {1, 6, 7, 5, 3}, 'Ы': {6, 7, 5, 3, 4, 2}, 'Ь': {6, 7, 5, 3, 4},
	'Э': {1, 2, 7, 3, 4}, 'Ю': {1, 2, 3, 4, 5, 6}, 'Я': {1, 2, 3, 5, 6, 7},
	'Ф': {1, 2, 3, 5, 6, 7}, 'Ц': {6, 5, 4, 3, 2, 7},
	// знаки
	' ': {}, '-': {7}, '_': {4}, '!': {2, 4}, '?': {1, 2, 7, 5, 4},
	'.': {4}, ':': {4}, '°': {1, 2, 7},
}

// CanDisplay — true, если весь текст можно показать (как is_displayable).
func CanDisplay(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if _, ok := Font[r]; !ok {
			return false
		}
	}
	return true
}

// --- Цвета (имена — как ключи RGB_COLORS в Python) ---

var RGBColors = map[string]RGB{
	"Красный":    {255, 0, 0},
	"Оранжевый":  {255, 127, 0},
	"Желтый":     {255, 255, 0},
	"Зеленый":    {0, 255, 0},
	"Синий":      {0, 0, 255},
	"Голубой":    {0, 191, 255},
	"Фиолетовый": {148, 0, 211},
	"Розовый":    {255, 105, 180},
	"Белый":      {255, 255, 255},
	"Черный":     {0, 0, 0},
}

// Порядок = порядок кнопок на клавиатуре (3 в ряд).
var colorOrder = []string{
	"Красный", "Оранжевый", "Желтый",
	"Зеленый", "Синий", "Фиолетовый",
	"Черный", "Белый", "Розовый",
}

var emojiByName = map[string]string{
	"Красный": "❤️", "Оранжевый": "🧡", "Желтый": "💛",
	"Зеленый": "💚", "Синий": "💙", "Фиолетовый": "💜",
	"Черный": "🖤", "Белый": "🤍", "Розовый": "🩷",
}

const (
	EmojiRainbow      = "🌈"
	EmojiTemperature  = "🌡"
	EmojiSlots        = "🎰"
)

// ColorEmojis — эмодзи кнопок цветов (порядок клавиатуры).
func ColorEmojis() []string {
	out := make([]string, 0, len(colorOrder))
	for _, n := range colorOrder {
		out = append(out, emojiByName[n])
	}
	return out
}

// EmojiColor — RGB по эмодзи кнопки (обратный lookups COLORS+RGB_COLORS).
func EmojiColor(emoji string) (RGB, bool) {
	for name, e := range emojiByName {
		if e == emoji {
			c, ok := RGBColors[name]
			return c, ok
		}
	}
	return RGB{}, false
}

// RandomColor — случайный цвет из палитры, кроме чёрного
// (логика get_random_color_from_existing).
func RandomColor() RGB {
	for {
		c := RGBColors[colorOrder[rand.Intn(len(colorOrder))]]
		if c != Black {
			return c
		}
	}
}

// --- Слот-машина ---

var SlotNames = []string{"bar", "grapes", "lemon", "seven"}

type SlotSeg struct {
	Seg   int
	Color RGB
}

var slotGold, slotPurple, slotYellow, slotRed = RGB{255, 215, 0}, RGB{148, 0, 211}, RGB{255, 255, 0}, RGB{255, 0, 0}

var SlotGlyphs = map[string][]SlotSeg{
	"bar":    {{1, slotGold}, {2, slotGold}, {3, slotGold}, {4, slotGold}, {5, slotGold}, {6, slotGold}, {7, slotGold}},
	"grapes": {{1, slotPurple}, {2, slotPurple}, {3, slotPurple}, {4, slotPurple}, {5, slotPurple}, {6, slotPurple}},
	"lemon":  {{2, slotYellow}, {3, slotYellow}, {4, slotYellow}, {5, slotYellow}, {6, slotYellow}},
	"seven":  {{1, slotRed}, {2, slotRed}, {3, slotRed}},
}

// Reels раскладывает значение кубика Telegram (1..64) на 4 барабана
// по 2 бита: индекс 0 = левый барабан.
func Reels(value int) [4]string {
	v := value - 1
	if v < 0 {
		v = 0
	}
	var out [4]string
	for i := 0; i < 4; i++ {
		out[i] = SlotNames[(v>>(2*(3-i)))&3]
	}
	return out
}

// IsJackpot — все четыре барабана совпали (включая 64 = четыре семёрки).
func IsJackpot(value int) bool {
	r := Reels(value)
	for _, s := range r[1:] {
		if s != r[0] {
			return false
		}
	}
	return true
}