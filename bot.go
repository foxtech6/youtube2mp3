package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	stats   *Stats
	updates tgbotapi.UpdatesChannel
}

type Stats struct {
	TotalRequests       int64
	SuccessfulDownloads int64
	FailedDownloads     int64
	StartTime           time.Time
}

func NewBot(token string) (*Bot, error) {
	// Ініціалізуємо Telegram API
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("помилка ініціалізації Telegram API: %w", err)
	}

	bot := &Bot{
		api: api,
		stats: &Stats{
			StartTime: time.Now(),
		},
	}

	logrus.Infof("✅ Бот авторизовано як @%s", api.Self.UserName)
	return bot, nil
}

func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	b.updates = b.api.GetUpdatesChan(u)

	for update := range b.updates {
		if update.Message != nil {
			go b.handleMessage(update.Message)
		}
	}
}

func (b *Bot) Stop() {
	b.api.StopReceivingUpdates()
	b.cleanupTempFiles()
}

func (b *Bot) handleMessage(message *tgbotapi.Message) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorf("Panic в handleMessage: %v", r)
		}
	}()

	chatID := message.Chat.ID
	userID := message.From.ID
	userName := message.From.UserName

	logrus.Infof("📨 Повідомлення від %s (%d): %s", userName, userID, message.Text)

	// Обробляємо команди
	if message.IsCommand() {
		switch message.Command() {
		case "start":
			b.handleStart(chatID, message.From.FirstName)
		case "help":
			b.handleHelp(chatID)
		case "stats":
			b.handleStats(chatID)
		default:
			b.sendMessage(chatID, "❌ Невідома команда. Використовуйте /help для довідки.")
		}
		return
	}

	// Обробляємо URL
	if message.Text != "" {
		b.handleURL(chatID, userID, message.Text)
	}
}

func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"

	if _, err := b.api.Send(msg); err != nil {
		logrus.Errorf("Помилка надсилання повідомлення: %v", err)
	}
}

func (b *Bot) editMessage(chatID int64, messageID int, text string) {
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = "Markdown"

	if _, err := b.api.Send(edit); err != nil {
		logrus.Errorf("Помилка редагування повідомлення: %v", err)
	}
}

func (b *Bot) cleanupTempFiles() {
	pattern := "/tmp/audio_*.mp3"
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return
	}

	for _, file := range matches {
		os.Remove(file)
	}

	logrus.Infof("🧹 Видалено %d тимчасових файлів", len(matches))
}

// Перевірка YouTube URL
func isValidYouTubeURL(url string) bool {
	patterns := []string{
		`^https?://(www\.)?youtube\.com/watch\?v=[a-zA-Z0-9_-]{11}`,
		`^https?://(www\.)?youtu\.be/[a-zA-Z0-9_-]{11}`,
		`^https?://(www\.)?m\.youtube\.com/watch\?v=[a-zA-Z0-9_-]{11}`,
	}

	for _, pattern := range patterns {
		matched, _ := regexp.MatchString(pattern, url)
		if matched {
			return true
		}
	}
	return false
}

// Завантаження відео через yt-dlp
func (b *Bot) downloadVideo(url string) (string, string, error) {
	// Генеруємо унікальну назву файлу
	filename := fmt.Sprintf("audio_%d", time.Now().Unix())
	outputPath := fmt.Sprintf("/tmp/%s.%%(ext)s", filename)

	// Команда yt-dlp для завантаження аудіо
	cmd := exec.Command("yt-dlp",
		"--extract-audio",
		"--audio-format", "mp3",
		"--audio-quality", "192K",
		"--output", outputPath,
		"--no-playlist",
		url)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("помилка yt-dlp: %s", string(output))
	}

	// Шукаємо створений файл
	mp3File := fmt.Sprintf("/tmp/%s.mp3", filename)
	if _, err := os.Stat(mp3File); os.IsNotExist(err) {
		return "", "", fmt.Errorf("файл не створено")
	}

	// Отримуємо назву відео з виводу yt-dlp
	lines := strings.Split(string(output), "\n")
	title := "Unknown"
	for _, line := range lines {
		if strings.Contains(line, "[download]") && strings.Contains(line, "Destination:") {
			parts := strings.Split(line, "/")
			if len(parts) > 0 {
				lastPart := parts[len(parts)-1]
				title = strings.TrimSuffix(lastPart, ".mp3")
			}
			break
		}
	}

	return mp3File, title, nil
}

// Отримання інформації про відео
func (b *Bot) getVideoInfo(url string) (string, int, error) {
	cmd := exec.Command("yt-dlp",
		"--get-title",
		"--get-duration",
		"--no-playlist",
		url)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", 0, fmt.Errorf("помилка отримання інформації: %s", string(output))
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return "", 0, fmt.Errorf("неповна інформація про відео")
	}

	title := lines[0]
	durationStr := lines[1]

	// Парсимо тривалість (format: MM:SS або HH:MM:SS)
	duration := parseDuration(durationStr)

	return title, duration, nil
}

func parseDuration(durationStr string) int {
	parts := strings.Split(durationStr, ":")
	if len(parts) == 2 {
		// MM:SS
		minutes := parseInt(parts[0])
		seconds := parseInt(parts[1])
		return minutes*60 + seconds
	} else if len(parts) == 3 {
		// HH:MM:SS
		hours := parseInt(parts[0])
		minutes := parseInt(parts[1])
		seconds := parseInt(parts[2])
		return hours*3600 + minutes*60 + seconds
	}
	return 0
}

func parseInt(s string) int {
	result := 0
	for _, char := range s {
		if char >= '0' && char <= '9' {
			result = result*10 + int(char-'0')
		}
	}
	return result
}

func formatDuration(seconds int) string {
	minutes := seconds / 60
	secs := seconds % 60
	return fmt.Sprintf("%d:%02d", minutes, secs)
}
