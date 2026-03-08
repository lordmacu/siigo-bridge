package telegram

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"siigo-common/config"
	"strings"
	"time"
)

type Bot struct {
	cfg      *config.TelegramConfig
	handlers map[string]CommandHandler
	offset   int64
	stopCh   chan struct{}
}

type CommandHandler func(args string) string

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

type tgResponse struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

func New(cfg *config.TelegramConfig) *Bot {
	return &Bot{
		cfg:      cfg,
		handlers: make(map[string]CommandHandler),
		stopCh:   make(chan struct{}),
	}
}

func (b *Bot) IsEnabled() bool {
	return b.cfg != nil && b.cfg.Enabled && b.cfg.BotToken != "" && b.cfg.ChatID != 0
}

func (b *Bot) RegisterCommand(cmd string, handler CommandHandler) {
	b.handlers[cmd] = handler
}

func (b *Bot) Send(msg string) {
	if !b.IsEnabled() {
		return
	}
	go b.sendSync(msg)
}

func (b *Bot) sendSync(msg string) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.cfg.BotToken)
	resp, err := http.PostForm(apiURL, url.Values{
		"chat_id":    {fmt.Sprintf("%d", b.cfg.ChatID)},
		"text":       {msg},
		"parse_mode": {"HTML"},
	})
	if err != nil {
		log.Printf("[Telegram] Error enviando: %v", err)
		return
	}
	resp.Body.Close()
}

// StartPolling begins listening for commands in background
func (b *Bot) StartPolling() {
	if !b.IsEnabled() {
		return
	}
	go b.pollLoop()
}

func (b *Bot) StopPolling() {
	select {
	case b.stopCh <- struct{}{}:
	default:
	}
}

func (b *Bot) pollLoop() {
	log.Println("[Telegram] Polling iniciado")
	client := &http.Client{Timeout: 35 * time.Second}

	for {
		select {
		case <-b.stopCh:
			log.Println("[Telegram] Polling detenido")
			return
		default:
		}

		updates := b.getUpdates(client)
		for _, u := range updates {
			b.offset = u.UpdateID + 1
			if u.Message == nil || u.Message.Chat.ID != b.cfg.ChatID {
				continue
			}
			text := strings.TrimSpace(u.Message.Text)
			if !strings.HasPrefix(text, "/") {
				continue
			}
			parts := strings.SplitN(text, " ", 2)
			cmd := strings.Split(parts[0], "@")[0] // remove @botname
			args := ""
			if len(parts) > 1 {
				args = parts[1]
			}

			if handler, ok := b.handlers[cmd]; ok {
				reply := handler(args)
				if reply != "" {
					b.sendSync(reply)
				}
			} else if cmd == "/help" || cmd == "/start" {
				b.sendSync(b.helpMessage())
			} else {
				b.sendSync("Comando no reconocido. Usa /help para ver los disponibles.")
			}
		}
	}
}

func (b *Bot) getUpdates(client *http.Client) []tgUpdate {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=30", b.cfg.BotToken, b.offset)
	resp, err := client.Get(apiURL)
	if err != nil {
		time.Sleep(5 * time.Second)
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var result tgResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil
	}
	return result.Result
}

func (b *Bot) helpMessage() string {
	cmds := []string{
		"/status - Estado del servidor",
		"/stats - Conteos por modulo",
		"/errors - Resumen de errores",
		"/sync - Sincronizar ahora",
		"/pause - Pausar sync",
		"/resume - Reanudar sync",
		"/retry - Reintentar errores",
		"/url - URLs actuales",
		"/logs - Ultimos logs",
		"/health - Health check",
		"/send-resume - Reactivar envio (tras auto-pausa)",
		"/exec {pin} {cmd} - Ejecutar comando",
		"/claude - Iniciar Claude remoto",
		"/help - Esta ayuda",
	}
	return "🤖 <b>Siigo Sync Bot</b>\n\nComandos disponibles:\n\n" + strings.Join(cmds, "\n")
}

// ==================== NOTIFICATIONS ====================

func (b *Bot) NotifyServerStarted(localURL string) {
	b.Send(fmt.Sprintf("🟢 <b>Servidor iniciado</b>\n\n🖥 %s", localURL))
}

func (b *Bot) NotifySyncCycleComplete(adds, edits, errors, pending int) {
	if adds == 0 && edits == 0 && errors == 0 {
		return
	}
	b.Send(fmt.Sprintf("🔄 <b>Sync completado</b>\n\n✅ Nuevos: %d\n📝 Editados: %d\n❌ Errores: %d\n⏳ Pendientes: %d", adds, edits, errors, pending))
}

func (b *Bot) NotifySyncErrors(table string, count int, lastError string) {
	if count == 0 {
		return
	}
	b.Send(fmt.Sprintf("⚠️ <b>Errores en %s</b>\n\n%d registros fallaron\nUltimo error: <code>%s</code>", table, count, truncate(lastError, 200)))
}

func (b *Bot) NotifyLoginFailed(apiURL string, err string) {
	b.Send(fmt.Sprintf("🔴 <b>Login fallido</b>\n\n🌐 %s\n❌ %s", apiURL, truncate(err, 200)))
}

func (b *Bot) NotifyMaxRetriesExhausted(table string, count int) {
	if count == 0 {
		return
	}
	b.Send(fmt.Sprintf("🚨 <b>Reintentos agotados</b>\n\n📋 %s: %d registros alcanzaron el maximo de reintentos", table, count))
}

func (b *Bot) NotifyDBCleared() {
	b.Send("🗑 <b>Base de datos vaciada</b>\n\nUn usuario vacio todas las tablas de SQLite.")
}

func (b *Bot) NotifyChangesDetected(table string, adds, edits, deletes int) {
	if adds == 0 && edits == 0 && deletes == 0 {
		return
	}
	b.Send(fmt.Sprintf("📊 <b>Cambios en %s</b>\n\n➕ %d nuevos\n📝 %d editados\n🗑 %d eliminados", table, adds, edits, deletes))
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
