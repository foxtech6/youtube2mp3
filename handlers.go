package main

import (
	"fmt"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/sirupsen/logrus"
)

func (b *Bot) handleStart(chatID int64, firstName string) {
	text := fmt.Sprintf(`👋 Привіт, **%s**!

🎵 **YouTube to MP3 Конвертер**

Я допоможу тобі швидко конвертувати YouTube відео в MP3!

**🚀 Як користуватися:**
1. Надішли мені посилання на YouTube відео
2. Я завантажу та конвертую його в MP3
3. Надішлю тобі готовий аудіо файл

**📝 Підтримувані посилання:**
• youtube.com/watch?v=...
• youtu.be/...
• m.youtube.com/watch?v=...

**⚙️ Команди:**
/start - Це повідомлення
/help - Детальна допомога  
/stats - Статистика бота

**⚠️ Обмеження:**
• Максимум: 10 хвилин
• Розмір: до 50 МБ
• Якість: 192 kbps

Надішли YouTube посилання! 🎶`, firstName)

	b.sendMessage(chatID, text)
}

func (b *Bot) handleHelp(chatID int64) {
	text := `🆘 **Детальна допомога**

**✅ Підтримувані формати:**
• Стандартні YouTube відео
• YouTube Shorts
• Мобільні посилання (m.youtube.com)

**📋 Приклади:**
• https://www.youtube.com/watch?v=dQw4w9WgXcQ
• https://youtu.be/dQw4w9WgXcQ
• https://m.youtube.com/watch?v=dQw4w9WgXcQ

**❌ Обмеження:**
• Тривалість: максимум 10 хвилин
• Розмір файлу: до 50 МБ
• Тільки публічні відео
• Без вікових обмежень

**🔧 Технічні деталі:**
• Формат: MP3
• Якість: 192 kbps
• Кодек: MPEG-1 Audio Layer III

**❓ Проблеми?**
1. Перевірте правильність посилання
2. Переконайтеся, що відео публічне
3. Спробуйте ще раз через кілька секунд

**🤖 Про бота:**
Використовує yt-dlp та FFmpeg для конвертації.
Всі файли видаляються після надсилання.`

	b.sendMessage(chatID, text)
}

func (b *Bot) handleStats(chatID int64) {
	uptime := time.Since(b.stats.StartTime)
	successRate := float64(0)
	if b.stats.TotalRequests > 0 {
		successRate = float64(b.stats.SuccessfulDownloads) / float64(b.stats.TotalRequests) * 100
	}

	text := fmt.Sprintf(`📊 **Статистика бота**

📈 **Загальна статистика:**
• Всього запитів: %d
• Успішних: %d
• Невдалих: %d
• Успішність: %.1f%%

⏰ **Час роботи:**
• Запущено: %s
• Працює: %s

🤖 **Версія:** 2.0 Go
⚡ **Статус:** Активний`,
		b.stats.TotalRequests,
		b.stats.SuccessfulDownloads,
		b.stats.FailedDownloads,
		successRate,
		b.stats.StartTime.Format("02.01.2006 15:04"),
		formatUptime(uptime))

	b.sendMessage(chatID, text)
}

func (b *Bot) handleURL(chatID int64, userID int64, url string) {
	b.stats.TotalRequests++

	// Перевіряємо URL
	if !isValidYouTubeURL(url) {
		b.sendMessage(chatID, `❌ **Невірне посилання!**

Надішліть посилання на YouTube у форматі:
• https://www.youtube.com/watch?v=...
• https://youtu.be/...
• https://m.youtube.com/watch?v=...

💡 **Підказка:** Скопіюйте посилання з YouTube додатка або браузера`)
		b.stats.FailedDownloads++
		return
	}

	// Надсилаємо повідомлення про початок обробки
	msg := tgbotapi.NewMessage(chatID, "🔍 Перевіряю відео...")
	msg.ParseMode = "Markdown"
	statusMsg, err := b.api.Send(msg)
	if err != nil {
		logrus.Errorf("Помилка надсилання статусного повідомлення: %v", err)
		return
	}

	// Отримуємо інформацію про відео
	title, duration, err := b.getVideoInfo(url)
	if err != nil {
		b.editMessage(chatID, statusMsg.MessageID,
			"❌ **Не вдалося отримати інформацію про відео**\\n\\nПеревірте посилання та спробуйте ще раз.")
		b.stats.FailedDownloads++
		return
	}

	// Перевіряємо тривалість (максимум 10 хвилин = 600 секунд)
	if duration > 600 {
		b.editMessage(chatID, statusMsg.MessageID,
			fmt.Sprintf("❌ **Відео занадто довге!**\\n\\n📊 Тривалість: %s\\n⏱️ Максимум: 10:00",
				formatDuration(duration)))
		b.stats.FailedDownloads++
		return
	}

	// Показуємо інформацію про відео
	infoText := fmt.Sprintf(`🎵 **%s**
⏱️ Тривалість: %s

🔄 Завантажую та конвертую...`,
		title,
		formatDuration(duration))

	b.editMessage(chatID, statusMsg.MessageID, infoText)

	// Завантажуємо аудіо
	audioFile, audioTitle, err := b.downloadVideo(url)
	if err != nil {
		b.editMessage(chatID, statusMsg.MessageID,
			fmt.Sprintf("❌ **Помилка завантаження**\\n\\n%s\\n\\nСпробуйте ще раз через кілька хвилин.", err.Error()))
		b.stats.FailedDownloads++
		return
	}

	// Перевіряємо розмір файлу
	fileInfo, err := os.Stat(audioFile)
	if err != nil {
		b.editMessage(chatID, statusMsg.MessageID, "❌ **Помилка читання файлу**")
		b.stats.FailedDownloads++
		return
	}

	fileSizeMB := float64(fileInfo.Size()) / (1024 * 1024)
	if fileSizeMB > 50 {
		os.Remove(audioFile)
		b.editMessage(chatID, statusMsg.MessageID,
			fmt.Sprintf("❌ **Файл занадто великий!**\\n\\n📊 Розмір: %.1f МБ\\n📏 Максимум: 50 МБ",
				fileSizeMB))
		b.stats.FailedDownloads++
		return
	}

	// Надсилаємо файл
	b.editMessage(chatID, statusMsg.MessageID, "📤 Надсилаю файл...")

	audio := tgbotapi.NewAudio(chatID, tgbotapi.FilePath(audioFile))
	audio.Title = audioTitle
	audio.Caption = fmt.Sprintf("🎵 **%s**\\n\\n📊 Розмір: %.1f МБ\\n🎧 Якість: 192 kbps",
		audioTitle, fileSizeMB)
	audio.ParseMode = "Markdown"

	if _, err := b.api.Send(audio); err != nil {
		logrus.Errorf("Помилка надсилання аудіо: %v", err)
		b.editMessage(chatID, statusMsg.MessageID, "❌ **Помилка надсилання файлу**")
		b.stats.FailedDownloads++
	} else {
		// Видаляємо повідомлення про прогрес
		deleteMsg := tgbotapi.NewDeleteMessage(chatID, statusMsg.MessageID)
		b.api.Send(deleteMsg)

		b.stats.SuccessfulDownloads++
		logrus.Infof("✅ Успішно конвертовано для користувача %d: %s", userID, audioTitle)
	}

	// Видаляємо тимчасовий файл
	os.Remove(audioFile)
}

func formatUptime(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	return fmt.Sprintf("%dг %dхв", hours, minutes)
}
