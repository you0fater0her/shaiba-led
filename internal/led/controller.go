package led

import (
	"context"
	"sync"
	"time"
)

const (
	Positions         = 4
	PixelsPerSymbol   = 98
	SegmentsPerSymbol = 7
	LedsPerSegment    = 14
)

// frameInterval — пауза между «подсветкой очередного LED» в анимации.
// В Python это был per-show() вызов (~30 fps, мерцание на ws281x).
const frameInterval = 30 * time.Millisecond

// Controller — потокобезопасный владелец фреймбуфера ленты.
// Все обращения к буферу под mutex; Render всегда отправляет цельный кадр.
type Controller struct {
	strip Strip

	mu      sync.Mutex
	buf     []RGB
	lit     []bool
	color   RGB
	rainbow bool
}

func NewController(s Strip) *Controller {
	return &Controller{
		strip: s,
		buf:   make([]RGB, s.Count()),
		lit:   make([]bool, s.Count()),
	}
}

func (c *Controller) SetColor(rgb RGB) {
	c.mu.Lock()
	c.color = rgb
	c.mu.Unlock()
}

func (c *Controller) Color() RGB {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.color
}

func (c *Controller) SetRainbow(on bool) {
	c.mu.Lock()
	c.rainbow = on
	c.mu.Unlock()
}

func (c *Controller) ToggleRainbow() bool {
	c.mu.Lock()
	c.rainbow = !c.rainbow
	on := c.rainbow
	c.mu.Unlock()
	return on
}

func (c *Controller) Rainbow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rainbow
}

// dynamicColor — цвет пикселя с учётом текущего режима (радуга/фикс).
func (c *Controller) dynamicColor(i int) RGB {
	if c.rainbow {
		return RainbowColor(i, len(c.buf), rainbowOffset())
	}
	return c.color
}

func (c *Controller) render() {
	for i, col := range c.buf {
		c.strip.SetPixel(i, col)
	}
	_ = c.strip.Render() // ошибки рендера не роняем, лента не критична
}

// Clear гасит всю ленту.
func (c *Controller) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	clear(c.buf)
	clear(c.lit)
	c.render()
}

// offset позиции: 0 — правая, 3 — левая (как _get_symbol_offset в Python).
func (c *Controller) offset(pos int) int {
	return PixelsPerSymbol * (Positions - 1 - pos)
}

func (c *Controller) clearPosition(pos int) {
	off := c.offset(pos)
	for i := off; i < off+PixelsPerSymbol; i++ {
		c.buf[i] = Black
		c.lit[i] = false
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// drawSegment рисует один сегмент; override != nil — фиксированный цвет
// (для слот-символов), иначе динамический (радуга/текущий).
func (c *Controller) drawSegment(pos, seg int, override *RGB, animate bool, ctx context.Context) error {
	off := c.offset(pos) + (seg-1)*LedsPerSegment
	for j := 0; j < LedsPerSegment; j++ {
		idx := off + j
		col := c.dynamicColor(idx)
		if override != nil {
			col = *override
		}
		c.buf[idx] = col
		c.lit[idx] = true
		if animate {
			c.mu.Unlock() // рендер без блокировки буфера не нужен, но держим кадр консистентным
			_ = c.strip.Render()
			err := sleepCtx(ctx, frameInterval)
			c.mu.Lock()
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// ShowSymbol показывает руну на позиции (0=справа). animate=true — побегушки.
func (c *Controller) ShowSymbol(pos int, r rune, animate bool, ctx context.Context) error {
	segs, ok := Font[r]
	if !ok {
		return nil // неизвестный символ = пусто, как в Python (print + return)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearPosition(pos)
	for _, s := range segs {
		if err := c.drawSegment(pos, s, nil, animate, ctx); err != nil {
			return err
		}
	}
	c.render()
	return nil
}

// ShowSlotSymbol — цветной символ слот-машины.
func (c *Controller) ShowSlotSymbol(pos int, name string, animate bool, ctx context.Context) error {
	segs, ok := SlotGlyphs[name]
	if !ok {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clearPosition(pos)
	for _, ss := range segs {
		col := ss.Color
		if err := c.drawSegment(pos, ss.Seg, &col, animate, ctx); err != nil {
			return err
		}
	}
	c.render()
	return nil
}

// ShowString выводит строку слева направо (<=4 символов) без скролла.
func (c *Controller) ShowString(s string, animate bool, ctx context.Context) error {
	runes := []rune(s)
	for i := 0; i < Positions; i++ {
		var r rune = ' '
		if i < len(runes) {
			r = runes[i]
		}
		if err := c.ShowSymbol(Positions-1-i, r, animate, ctx); err != nil {
			return err
		}
	}
	return nil
}

// RecolorLit перекрашивает уже горящие пиксели (плавная смена цвета/радуги
// без перерисовки структуры — аналог update_colors).
func (c *Controller) RecolorLit() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.buf {
		if c.lit[i] {
			c.buf[i] = c.dynamicColor(i)
		}
	}
	c.render()
}

// FillRainbow заливает всю ленту радужной волной с фазой offset.
func (c *Controller) FillRainbow(offset int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range c.buf {
		c.buf[i] = RainbowColor(i, len(c.buf), offset)
		c.lit[i] = true
	}
	c.render()
}