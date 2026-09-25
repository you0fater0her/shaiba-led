package botapi

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/net/proxy"

	"ledstrip/internal/config"
	"ledstrip/internal/display"
	"ledstrip/internal/led"
	"ledstrip/internal/storage"
)

const rateLimitWindow = 5 * time.Second

type limiter struct {
	mu sync.Mutex
	m  map[int64]time.Time
}

func (l *limiter) allow(id int64) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.m) > 10_000 { // страховка от роста, как в Python не было
		l.m = map[int64]time.Time{}
	}
	now := time.Now()
	if last, ok := l.m[id]; ok && now.Sub(last) < rateLimitWindow {
		return false
	}
	l.m[id] = now
	return true
}

type Bot struct {
	api     *tgbotapi.BotAPI
	eng     *display.Engine
	store   *storage.Store
	cfg     *config.Config
	limiter limiter
}

func New(cfg *config.Config, eng *display.Engine, store *storage.Store) (*Bot, error) {
	client, err := telegramHTTPClient(cfg.Telegram.Proxy)
	if err != nil {
		return nil, err
	}
	api, err := tgbotapi.NewBotAPI(cfg.Telegram.Token)
	if err != nil {
		return nil, err
	}
	api.Client = client
	log.Printf("telegram: authorized as @%s", api.Self.UserName)
	return &Bot{
		api: api, eng: eng, store: store, cfg: cfg,
		limiter: limiter{m: map[int64]time.Time{}},
	}, nil
}

// telegramHTTPClient — direct / http(s) proxy / socks5 (аналог session.py).
func telegramHTTPClient(proxyURL string) (*http.Client, error) {
	tr := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("TELEGRAM_PROXY_URL: %w", err)
		}
		switch u.Scheme {
		case "http", "https":
			tr.Proxy = http.ProxyURL(u) // https-прокси = CONNECT поверх TLS, Go умеет сам
		case "socks5":
			var auth *proxy.Auth
			if u.User != nil {
				p, _ := u.User.Password()
				auth = &proxy.Auth{User: u.User.Username(), Password: p}
			}
			d, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
			if err != nil {
				return nil, err
			}
			tr.Proxy = nil
			tr.DialContext = func(ctx context.Context, _, addr string) (net.Conn, error) {
				type ctxDialer interface {
					DialContext(context.Context, string, string) (net.Conn, error)
				}
				if cd, ok := d.(ctxDialer); ok {
					return cd.DialContext(ctx, "tcp", addr)
				}
				return d.Dial("tcp", addr)
			}
		default:
			return nil, fmt.Errorf("unsupported proxy scheme: %q", u.Scheme)
		}
	}
	return &http.Client{Transport: tr, Timeout: 45 * time.Second}, nil
}

// Run — long-poll с экспоненциальным backoff (аналог _resilient_polling).
func (b *Bot) Run(ctx context.Context) error {
	retry := 5 * time.Second
	const maxRetry = 5 * time.Minute

	for {
		if ctx.Err() != nil {
			return nil
		}
		conf := tgbotapi.NewUpdate(0)
		conf.Timeout = 25
		updates := b.api.GetUpdatesChan(conf)

	retryLoop:
		for {
			select {
			case <-ctx.Done():
				b.api.StopReceivingUpdates()
				return nil
			case u, ok := <-updates:
				if !ok { // канал закрыт — GetUpdatesChan внутри перезапускается
					break retryLoop
				}
				b.safeHandle(ctx, u)
			}
		}

		log.Printf("telegram: polling interrupted, retry in %s", retry)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(retry):
		}
		retry *= 2
		if retry > maxRetry {
			retry = maxRetry
		}
	}
}

func (b *Bot) safeHandle(ctx context.Context, u tgbotapi.Update) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in update handler: %v", r)
		}
	}()
	b.handle(ctx, u)
}

func (b *Bot) handle(ctx context.Context, u tgbotapi.Update) {
	msg := u.Message
	if msg == nil || msg.From == nil {
		return
	}
	if msg.Dice != nil {
		b.handleDice(ctx, msg)
		return
	}
	if msg.Text == "" {
		return
	}
	if msg.IsCommand() {
		b.handleCommand(ctx, msg)
		return
	}
	b.handleText(ctx, msg)
}

// --- Команды ---

func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	userID := msg.From.ID
	switch msg.Command() {
	case "start":
		b.reply(ctx, msg, "Привет! 👋\n\n"+
			"Отправляй мне эмодзи-сердечки и я буду менять цвет ленты "+
			"в цвет отправленного сердечка.\n\n"+
			"🌈 — радуга\n🌡 — температура")

	case "help":
		text := "Доступные команды:\n\n" +
			"/start — начать работу\n" +
			"/help — эта справка\n\n" +
			"Отправь сердечко чтобы изменить цвет, или напиши текст (только для админов)."
		if b.store.IsAdmin(userID) {
			text += "\n\n👑 Команды админа:\n" +
				"/set_new_year_date <YYYY-MM-DD HH:MM:SS> — установить дату Нового года"
		}
		if b.store.IsSuperAdmin(userID) {
			text += "\n\n👑 Команды супер-админа:\n" +
				"/add_admin <user_id> — добавить админа\n" +
				"/remove_admin <user_id> — удалить админа\n" +
				"/list_admins — список админов"
		}
		b.reply(ctx, msg, text)

	case "add_admin", "remove_admin", "list_admins":
		if !b.store.IsSuperAdmin(userID) {
			b.reply(ctx, msg, "⛔ Только супер-админ может управлять админами")
			return
		}
		b.handleAdminMgmt(ctx, msg)

	case "set_new_year_date":
		if !b.store.IsAdmin(userID) {
			b.reply(ctx, msg, "⛔ Только администраторы могут изменять дату Нового года")
			return
		}
		arg := msg.CommandArguments()
		t, err := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(arg), time.Local)
		if err != nil {
			b.reply(ctx, msg, "❌ Неверный формат даты.\n\n"+
				"Используйте формат: YYYY-MM-DD HH:MM:SS\nПример: 2026-01-01 00:00:00")
			return
		}
		if !b.eng.SetNewYearDate(t) {
			b.reply(ctx, msg, "Подожди пару секунд ⏳")
			return
		}
		b.reply(ctx, msg, "✅ Дата Нового года установлена: "+
			t.Format("2006-01-02 15:04:05")+
			"\n\nИзменения применятся автоматически в следующей итерации цикла.")
	}
}

func (b *Bot) handleAdminMgmt(ctx context.Context, msg *tgbotapi.Message) {
	args := strings.Fields(msg.CommandArguments())
	switch msg.Command() {
	case "list_admins":
		admins := b.store.ListAdmins()
		if len(admins) == 0 {
			b.reply(ctx, msg, "ℹ️ Список админов пуст (кроме супер-админа)")
			return
		}
		var sb strings.Builder
		sb.WriteString("👥 Список админов:\n\n")
		for _, id := range admins {
			fmt.Fprintf(&sb, "• %d\n", id)
		}
		b.reply(ctx, msg, sb.String())
	default:
		if len(args) < 1 {
			b.reply(ctx, msg, "Использование: /"+msg.Command()+" <user_id>")
			return
		}
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			b.reply(ctx, msg, "❌ Неверный формат user_id. Должно быть число.")
			return
		}
		if msg.Command() == "add_admin" {
			if b.store.AddAdmin(id) {
				b.reply(ctx, msg, fmt.Sprintf("✅ Админ %d добавлен", id))
			} else {
				b.reply(ctx, msg, fmt.Sprintf("ℹ️ Пользователь %d уже является админом", id))
			}
		} else {
			if b.store.RemoveAdmin(id) {
				b.reply(ctx, msg, fmt.Sprintf("✅ Админ %d удалён", id))
			} else {
				b.reply(ctx, msg, fmt.Sprintf("ℹ️ Пользователь %d не найден в списке админов", id))
			}
		}
	}
}

// --- Текст и медиа ---

func (b *Bot) handleText(ctx context.Context, msg *tgbotapi.Message) {
	userID := msg.From.ID
	text := msg.Text
	isAdmin := b.store.IsAdmin(userID)

	if b.eng.CountdownProtected() && !isAdmin {
		b.reply(ctx, msg, "🎄 Обратный отсчёт! Подожди немного...")
		return
	}
	if !b.limiter.allow(userID) {
		b.reply(ctx, msg, "Попробуй позже ⏳")
		return
	}

	// Режимы (только админы)
	switch text {
	case "main", "user", "fight":
		if !isAdmin {
			b.answer(ctx, msg, "Смена режима доступна только администраторам")
			return
		}
		var m display.Mode
		switch text {
		case "main":
			m = display.ModeMain
		case "user":
			m = display.ModeUser
		case "fight":
			m = display.ModeFight
		}
		b.eng.SetMode(m)
		b.answer(ctx, msg, "Режим: "+text)
		return
	}

	// Цвет по эмодзи
	if rgb, ok := led.EmojiColor(text); ok {
		b.eng.SetColor(rgb)
		b.store.SaveColor(rgb)
		b.answer(ctx, msg, "Цвет изменён ✨")
		b.notifyAdminAsync(ctx, msg, "изменил цвет "+text)
		return
	}

	// Радуга
	if text == led.EmojiRainbow {
		b.eng.ToggleRainbow()
		b.answer(ctx, msg, led.EmojiRainbow)
		return
	}

	// Температура
	if text == led.EmojiTemperature {
		if b.eng.Busy() {
			b.reply(ctx, msg, "Подожди пару секунд ⏳")
			return
		}
		if !b.eng.ShowTemperature() {
			b.reply(ctx, msg, "Подожди пару секунд ⏳")
			return
		}
		b.answer(ctx, msg, led.EmojiTemperature)
		return
	}

	// Слоты: кидаем кубик и показываем на ленте
	if text == led.EmojiSlots {
		if b.eng.Busy() {
			b.reply(ctx, msg, "Подожди пару секунд ⏳")
			return
		}
		diceMsg, err := b.api.Send(tgbotapi.DiceConfig{
			BaseChat: tgbotapi.BaseChat{ChatID: msg.Chat.ID},
			Emoji:    led.EmojiSlots,
		})
		if err != nil {
			log.Printf("send dice: %v", err)
			return
		}
		if diceMsg.Dice == nil {
			return
		}
		b.requestSlots(ctx, msg, diceMsg.Dice.Value)
		return
	}

	// Текст на ленту (админы, проверка символов шрифта)
	upper := strings.ToUpper(text)
	if led.CanDisplay(upper) {
		if !isAdmin {
			b.answer(ctx, msg, "Отправка текста доступна только администраторам")
			return
		}
		if b.eng.Busy() {
			b.reply(ctx, msg, "Подожди пару секунд ⏳")
			return
		}
		if !b.eng.ShowText(upper) {
			b.reply(ctx, msg, "Подожди пару секунд ⏳")
			return
		}
		b.notifyAdminAsync(ctx, msg, "показал текст: "+text)
		b.answer(ctx, msg, "Текст отправлен 📝")
		return
	}

	b.answer(ctx, msg, "Не понимаю. Отправь сердечко для смены цвета.")
}

// handleDice — входящий стикер 🎰 от пользователя.
func (b *Bot) handleDice(ctx context.Context, msg *tgbotapi.Message) {
	if msg.Dice == nil || msg.Dice.Emoji != led.EmojiSlots {
		return
	}
	userID := msg.From.ID

	if b.eng.CountdownProtected() && !b.store.IsAdmin(userID) {
		b.reply(ctx, msg, "🎄 Обратный отсчёт! Подожди немного...")
		return
	}
	if b.eng.Busy() {
		b.reply(ctx, msg, "Подожди пару секунд ⏳")
		return
	}
	if !b.limiter.allow(userID) {
		b.reply(ctx, msg, "Попробуй позже ⏳")
		return
	}
	b.requestSlots(ctx, msg, msg.Dice.Value)
}

func (b *Bot) requestSlots(ctx context.Context, msg *tgbotapi.Message, value int) {
	if !b.eng.RequestSlots(value) {
		b.reply(ctx, msg, "Подожди, слоты ещё крутятся! 🎰")
		return
	}
	b.notifyAdminAsync(ctx, msg, "крутит слоты 🎰")
	if led.IsJackpot(value) {
		b.reply(ctx, msg, "ДЖЕКПОТ!")
	}
}

// --- Отправка сообщений ---

func (b *Bot) answer(ctx context.Context, msg *tgbotapi.Message, text string) {
	m := tgbotapi.NewMessage(msg.Chat.ID, text)
	m.ReplyMarkup = mainKeyboard()
	b.send(ctx, m)
}

func (b *Bot) reply(ctx context.Context, msg *tgbotapi.Message, text string) {
	m := tgbotapi.NewMessage(msg.Chat.ID, text)
	m.ReplyToMessageID = msg.MessageID
	m.ReplyMarkup = mainKeyboard()
	b.send(ctx, m)
}

func (b *Bot) send(ctx context.Context, c tgbotapi.Chattable) {
	_, err := b.api.Send(c)
	if err != nil {
		log.Printf("send message: %v", err)
	}
}

// notifyAdminAsync — уведомление админу в отдельной горутине с таймаутом.
func (b *Bot) notifyAdminAsync(parent context.Context, msg *tgbotapi.Message, action string) {
	adminID := b.cfg.Telegram.AdminID
	if adminID == 0 || msg.From.ID == adminID {
		return
	}
	go func() {
		defer func() { _ = recover() }()
		name := msg.From.FirstName
		if msg.From.UserName != "" {
			name = "@" + msg.From.UserName
		}
		m := tgbotapi.NewMessage(adminID, fmt.Sprintf("%s %s", name, action))
		_, _ = b.api.Send(m)
	}()
}

// mainKeyboard — 3×3 цвета + ряд действий (как adjust(3,3,3,3) в Python).
func mainKeyboard() tgbotapi.ReplyKeyboardMarkup {
	var rows [][]tgbotapi.KeyboardButton
	emojis := led.ColorEmojis()
	for i := 0; i < len(emojis); i += 3 {
		row := []tgbotapi.KeyboardButton{}
		for _, e := range emojis[i:min(i+3, len(emojis))] {
			row = append(row, tgbotapi.NewKeyboardButton(e))
		}
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(led.EmojiTemperature),
		tgbotapi.NewKeyboardButton(led.EmojiRainbow),
		tgbotapi.NewKeyboardButton(led.EmojiSlots),
	))
	kb := tgbotapi.NewReplyKeyboard(rows...)
	kb.ResizeKeyboard = true
	return kb
}