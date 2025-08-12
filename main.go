package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/sirupsen/logrus"
)

func main() {
	// Налаштування логування
	logrus.SetLevel(logrus.InfoLevel)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	// Перевіряємо токен
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("❌ TELEGRAM_BOT_TOKEN не встановлено")
	}

	// Ініціалізуємо бота
	bot, err := NewBot(token)
	if err != nil {
		log.Fatal("❌ Помилка ініціалізації бота:", err)
	}

	logrus.Info("🚀 YouTube to MP3 бот запускається...")

	// Запускаємо бота
	go bot.Start()

	// Очікуємо сигнал завершення
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	<-c
	logrus.Info("🛑 Отримано сигнал завершення")
	bot.Stop()
	logrus.Info("✅ Бот зупинено")
}
