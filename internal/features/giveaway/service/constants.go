package service

import "time"

const (
	// Общие константы для обработки розыгрышей
	MaxConcurrentProcessing = 10               // Максимальное количество одновременно обрабатываемых розыгрышей
	ProcessingTimeout       = 2 * time.Minute  // Таймаут для обработки одного розыгрыша
	LockTimeout             = 30 * time.Second // Таймаут для блокировки
	CheckInterval           = 10 * time.Second // Интервал проверки розыгрышей
	CleanupInterval         = 30 * time.Minute // Интервал очистки несогласованных данных
	MaxRetries              = 3                // Максимальное количество попыток обработки
	RetryDelay              = 5 * time.Second  // Задержка между попытками
)
