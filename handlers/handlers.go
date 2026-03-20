package handlers

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/adkhorst/planbot/database"
	"github.com/adkhorst/planbot/googlecal"
	"github.com/adkhorst/planbot/models"
	"github.com/adkhorst/planbot/scheduler"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/oauth2"
)

// Bot context for handlers
type BotHandler struct {
	bot *tgbotapi.BotAPI
}

// NewBotHandler creates a new bot handler
func NewBotHandler(bot *tgbotapi.BotAPI) *BotHandler {
	return &BotHandler{bot: bot}
}

// HandleUpdate processes incoming updates
func (h *BotHandler) HandleUpdate(update tgbotapi.Update) {
	if update.Message == nil {
		return
	}

	msg := update.Message
	log.Printf("[%s] %s", msg.From.UserName, msg.Text)

	// Check if it's a command
	if msg.IsCommand() {
		h.handleCommand(msg)
		return
	}

	// Regular message handling if needed
	h.sendMessage(msg.Chat.ID, "Используйте /help для списка команд")
}

// handleCommand routes commands to appropriate handlers
func (h *BotHandler) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		h.handleStart(msg)
	case "help":
		h.handleHelp(msg)
	case "addtask":
		h.handleAddTask(msg)
	case "mytasks":
		h.handleMyTasks(msg)
	case "schedule":
		h.handleSchedule(msg)
	case "today":
		h.handleToday(msg)
	case "week":
		h.handleWeek(msg)
	case "schedule_slots":
		h.handleScheduleSlots(msg)
	case "complete":
		h.handleComplete(msg)
	case "delete":
		h.handleDelete(msg)
	case "settings":
		h.handleSettings(msg)
	case "timezone":
		h.handleTimezone(msg)
	case "google_connect":
		h.handleGoogleConnect(msg)
	case "google_code":
		h.handleGoogleCode(msg)
	case "google_status":
		h.handleGoogleStatus(msg)
	default:
		h.sendMessage(msg.Chat.ID, "Неизвестная команда. Используйте /help")
	}
}

// handleStart handles /start command
func (h *BotHandler) handleStart(msg *tgbotapi.Message) {
	user, err := database.GetOrCreateUser(
		msg.From.ID,
		msg.From.UserName,
		msg.From.FirstName,
		msg.From.LastName,
	)
	if err != nil {
		log.Printf("Error creating user: %v", err)
		h.sendMessage(msg.Chat.ID, "Произошла ошибка при регистрации")
		return
	}

	welcomeMsg := fmt.Sprintf(`Привет, %s! 👋

Я - PlanBot, твой помощник в планировании задач.

Я помогу тебе распределить задачи по дням с учётом:
• Времени, необходимого на каждую задачу
• Приоритетов
• Дедлайнов
• Твоей дневной нагрузки

Используй /help чтобы увидеть все команды.`, user.FirstName)

	h.sendMessage(msg.Chat.ID, welcomeMsg)
}

// handleHelp handles /help command
func (h *BotHandler) handleHelp(msg *tgbotapi.Message) {
	helpText := `📋 Доступные команды:

/addtask - Добавить новую задачу
Формат: /addtask Название | часы | приоритет | дедлайн
Минимум: /addtask Задача | 2
Примеры:
/addtask Написать отчёт | 4 | 5 | 25.12.2025
/addtask Прочитать статью | 1.5 | 3

/mytasks - Показать все задачи
/schedule - Автоматически распланировать задачи по дням
/today - Показать расписание на сегодня
/week - Показать расписание на неделю
/schedule_slots - Экспериментальное планирование по временным слотам
/complete [ID] - Отметить задачу выполненной (по ID из /mytasks)
/delete [ID] - Удалить задачу
/settings - Настройки (часы в день, рабочие дни)
/timezone [имя_таймзоны] - Установить таймзону (например, Europe/Moscow)
/google_connect - Подключить Google Calendar (OAuth)
/google_code [код] - Завершить подключение Google Calendar
/google_status - Статус подключения Google Calendar

💡 Советы:
• Приоритет: целое число от 1 до 10 (10 = самый важный)
• Дедлайн необязателен
• Задачи автоматически распределяются по рабочим дням с учётом ваших настроек`

	h.sendMessage(msg.Chat.ID, helpText)
}

// handleAddTask handles /addtask command
func (h *BotHandler) handleAddTask(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "⚠️ Не удалось получить профиль пользователя.\nПопробуйте сначала выполнить команду /start.")
		return
	}

	// Parse arguments: title | hours | priority | deadline
	args := msg.CommandArguments()
	if args == "" {
		h.sendMessage(msg.Chat.ID, "❗️ Не указан текст задачи.\n\nФормат: /addtask Название | часы | приоритет | дедлайн\nПример: /addtask Написать отчёт | 4 | 5 | 25.12.2025\n\nМинимум: /addtask Задача | 2")
		return
	}

	parts := strings.Split(args, "|")
	if len(parts) < 2 {
		h.sendMessage(msg.Chat.ID, "❗️ Минимум нужно указать название и количество часов.\nПример: /addtask Задача | 2")
		return
	}

	title := strings.TrimSpace(parts[0])
	hoursStr := strings.TrimSpace(parts[1])

	hours, err := strconv.ParseFloat(hoursStr, 64)
	if err != nil || hours <= 0 {
		h.sendMessage(msg.Chat.ID, "⏱ Неверное количество часов.\nУкажите положительное число, например: 0.5, 1, 2.5")
		return
	}

	task := &models.Task{
		UserID:        user.ID,
		Title:         title,
		HoursRequired: hours,
		Priority:      5, // default priority
	}

	// Parse priority if provided
	if len(parts) > 2 {
		priorityStr := strings.TrimSpace(parts[2])
		priority, err := strconv.Atoi(priorityStr)
		if err != nil {
			h.sendMessage(msg.Chat.ID, "⭐️ Неверный формат приоритета.\nИспользуйте целое число от 1 до 10 (10 = самый важный).")
			return
		}
		if priority < 1 || priority > 10 {
			h.sendMessage(msg.Chat.ID, "⭐️ Приоритет должен быть от 1 до 10.\nНапример: 3 (низкий), 5 (средний), 8–10 (высокий).")
			return
		}
		task.Priority = priority
	}

	// Parse deadline if provided
	if len(parts) > 3 {
		deadlineStr := strings.TrimSpace(parts[3])
		deadline, err := parseDate(deadlineStr)
		if err != nil {
			h.sendMessage(msg.Chat.ID, "📅 Неверный формат дедлайна.\nДопустимые форматы дат: 25.12.2025, 25.12.25 или 2025-12-25.")
			return
		}
		task.Deadline = &deadline
	}

	// Save task
	err = database.CreateTask(task)
	if err != nil {
		log.Printf("Error creating task: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка при создании задачи")
		return
	}

	response := fmt.Sprintf("✅ Задача создана!\n\n📝 %s\n⏱ %g часов\n⭐️ Приоритет: %d",
		task.Title, task.HoursRequired, task.Priority)

	if task.Deadline != nil {
		response += fmt.Sprintf("\n📅 Дедлайн: %s", task.Deadline.Format("02.01.2006"))
	}

	response += "\n\nИспользуйте /schedule для планирования"

	h.sendMessage(msg.Chat.ID, response)
}

// handleMyTasks handles /mytasks command
func (h *BotHandler) handleMyTasks(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "Ошибка получения пользователя")
		return
	}

	tasks, err := database.GetUserTasks(user.ID)
	if err != nil {
		log.Printf("Error getting tasks: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка получения задач")
		return
	}

	if len(tasks) == 0 {
		h.sendMessage(msg.Chat.ID, "У вас пока нет задач. Используйте /addtask")
		return
	}

	response := "📋 Ваши задачи:\n\n"
	for _, task := range tasks {
		statusEmoji := getStatusEmoji(task.Status)
		response += fmt.Sprintf("%s ID:%d | %s\n⏱ %g ч | ⭐️ %d",
			statusEmoji, task.ID, task.Title, task.HoursRequired, task.Priority)

		if task.Deadline != nil {
			response += fmt.Sprintf(" | 📅 %s", task.Deadline.Format("02.01.2006"))
		}
		response += "\n\n"
	}

	h.sendMessage(msg.Chat.ID, response)
}

// handleSchedule handles /schedule command
func (h *BotHandler) handleSchedule(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "⚠️ Не удалось получить профиль пользователя.\nПопробуйте сначала выполнить команду /start.")
		return
	}

	// Hard rescheduling: plan for ALL active tasks (pending + already scheduled, etc.)
	tasks, err := database.GetActiveTasks(user.ID)
	if err != nil {
		log.Printf("Error getting active tasks: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка получения задач из базы.\nПопробуйте позже.")
		return
	}

	if len(tasks) == 0 {
		h.sendMessage(msg.Chat.ID, "Нет активных задач для планирования.\nДобавьте новую задачу через /addtask.")
		return
	}

	h.sendMessage(msg.Chat.ID, "🔄 Планирую задачи...")

	// Run scheduler starting from tomorrow (планируем всегда с завтрашнего дня)
	s := scheduler.NewScheduler(user, tasks)
	now := time.Now()
	if user.TimeZone != "" {
		if loc, err := time.LoadLocation(user.TimeZone); err == nil {
			now = now.In(loc)
		}
	}
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	result := s.Schedule(startDate)

	// Clear old schedules for these tasks
	taskIDs := make([]int64, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.ID
	}
	database.ClearTaskSchedules(taskIDs)

	// Save new schedules
	if len(result.DaySchedules) > 0 {
		err = database.SaveTaskSchedules(result.DaySchedules)
		if err != nil {
			log.Printf("Error saving schedules: %v", err)
			h.sendMessage(msg.Chat.ID, "Ошибка сохранения расписания")
			return
		}
	}

	// Ensure task statuses are consistent after "hard" reschedule:
	// - scheduled tasks stay/switch to 'scheduled' (SaveTaskSchedules does that)
	// - unscheduled tasks should return to 'pending' (they may previously be 'scheduled')
	for _, unscheduledID := range result.UnscheduledTasks {
		_ = database.UpdateTaskStatus(unscheduledID, "pending")
	}

	// Optional: export schedule to Google Calendar.
	if len(result.DaySchedules) > 0 {
		ctx := context.Background()

		// Используем только токен из БД (Google OAuth).
		if storedTok, err := database.GetGoogleToken(user.ID); err != nil {
			log.Printf("Error getting Google token from DB: %v", err)
		} else if storedTok != nil {
			cfg, err := googlecal.ConfigFromEnv()
			if err != nil {
				log.Printf("Error creating Google OAuth config: %v", err)
			} else {
				client, err := googlecal.NewWithStoredToken(ctx, cfg, storedTok, func(t *oauth2.Token) error {
					return database.SaveGoogleToken(user.ID, t)
				})
				if err != nil {
					log.Printf("Error creating Google Calendar client with stored token: %v", err)
				} else {
					if err := client.ExportDaySchedules(ctx, "primary", user, result.DaySchedules); err != nil {
						log.Printf("Error exporting schedule to Google Calendar (stored token): %v", err)
					}
				}
			}
		}
	}

	// Format response
	scheduledTasksCount := len(tasks) - len(result.UnscheduledTasks)
	response := "✅ Планирование завершено.\n\n"
	response += fmt.Sprintf("📊 Результат: %s\n", result.Message)
	response += fmt.Sprintf("📌 Запланировано задач: %d из %d\n", scheduledTasksCount, len(tasks))

	if len(result.DaySchedules) > 0 {
		response += "📅 Расписание:\n\n"
		for i, daySchedule := range result.DaySchedules {
			if i >= 7 { // Show only first week
				response += fmt.Sprintf("\n... и ещё %d дней", len(result.DaySchedules)-7)
				break
			}
			response += formatDaySchedule(daySchedule, user.DailyCapacity)
		}
	}

	if len(result.UnscheduledTasks) > 0 {
		response += fmt.Sprintf("\n\n⚠️ Не удалось запланировать %d задач(и)", len(result.UnscheduledTasks))
	}

	h.sendMessage(msg.Chat.ID, response)
}

// handleScheduleSlots handles /schedule_slots command (experimental slot-based planning, no DB writes)
func (h *BotHandler) handleScheduleSlots(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "⚠️ Не удалось получить профиль пользователя.\nПопробуйте сначала выполнить команду /start.")
		return
	}

	// Show slot preview for the same set as /schedule hard rescheduling.
	tasks, err := database.GetActiveTasks(user.ID)
	if err != nil {
		log.Printf("Error getting active tasks: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка получения задач из базы.\nПопробуйте позже.")
		return
	}

	if len(tasks) == 0 {
		h.sendMessage(msg.Chat.ID, "Нет задач для планирования.\nДобавьте новую задачу через /addtask.")
		return
	}

	h.sendMessage(msg.Chat.ID, "🧪 Экспериментальное планирование по временным слотам...\n(данные в БД не изменяются)")

	// Используем тот же планировщик, что и /schedule, чтобы результат по дням совпадал.
	s := scheduler.NewScheduler(user, tasks)
	now := time.Now()
	if user.TimeZone != "" {
		if loc, err := time.LoadLocation(user.TimeZone); err == nil {
			now = now.In(loc)
		}
	}
	startDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	planResult := s.Schedule(startDate)

	if len(planResult.DaySchedules) == 0 {
		h.sendMessage(msg.Chat.ID, "🔎 Нет расписания для отображения по слотам. Сначала добавьте задачи.")
		return
	}

	// Генерируем слоты и равномерно раскладываем внутри рабочего дня уже запланированные часы.
	slotScheduler := scheduler.NewSlotScheduler(user)
	slots := slotScheduler.BuildDailySlots(startDate)

	// Индекс слотов по дате
	slotsByDate := make(map[string][]*models.TimeSlot)
	for i := range slots {
		key := slots[i].Date.Format("2006-01-02")
		s := &slots[i]
		slotsByDate[key] = append(slotsByDate[key], s)
	}

	// Для каждого дня из плана равномерно заполняем слоты задачами этого дня.
	for _, day := range planResult.DaySchedules {
		dateKey := day.Date.Format("2006-01-02")
		daySlots := slotsByDate[dateKey]
		if len(daySlots) == 0 {
			continue
		}

		// Последовательно проходим задачи этого дня и распределяем их часы по доступным слотам.
		slotIdx := 0
		for _, t := range day.Tasks {
			remaining := t.HoursAllocated
			for remaining > 0 && slotIdx < len(daySlots) {
				slot := daySlots[slotIdx]
				free := slot.CapacityHours - slot.AllocatedHours
				if free <= 0 {
					slotIdx++
					continue
				}

				toAllocate := remaining
				if toAllocate > free {
					toAllocate = free
				}

				slot.AllocatedHours += toAllocate
				if toAllocate > 0 {
					idCopy := t.TaskID
					slot.TaskID = &idCopy
					if slot.Source == "" {
						slot.Source = "task"
					}
				}

				remaining -= toAllocate
				if slot.AllocatedHours >= slot.CapacityHours {
					slotIdx++
				}
			}
		}
	}

	// Теперь у нас есть заполненные слоты, сворачиваем их обратно в DaySchedule только для показа.
	type aggTask struct {
		info models.ScheduledTaskInfo
	}

	dayAgg := make(map[string]models.DaySchedule)

	for _, slot := range slots {
		if slot.TaskID == nil || slot.AllocatedHours <= 0 {
			continue
		}

		dateKey := slot.Date.Format("2006-01-02")
		ds, exists := dayAgg[dateKey]
		if !exists {
			ds = models.DaySchedule{
				Date:           slot.Date,
				Tasks:          []models.ScheduledTaskInfo{},
				TotalHours:     0,
				AvailableHours: user.DailyCapacity,
			}
		}

		// Ищем задачу в уже существующем списке
		found := false
		for i := range ds.Tasks {
			if ds.Tasks[i].TaskID == *slot.TaskID {
				ds.Tasks[i].HoursAllocated += slot.AllocatedHours
				found = true
				break
			}
		}
		if !found {
			// Берём базовую информацию из исходного плана
			var baseInfo *models.ScheduledTaskInfo
			for _, d := range planResult.DaySchedules {
				if d.Date.Format("2006-01-02") != dateKey {
					continue
				}
				for _, ti := range d.Tasks {
					if ti.TaskID == *slot.TaskID {
						copy := ti
						baseInfo = &copy
						break
					}
				}
				if baseInfo != nil {
					break
				}
			}
			if baseInfo == nil {
				// fallback: минимальная информация
				baseInfo = &models.ScheduledTaskInfo{
					TaskID: *slot.TaskID,
				}
			}
			baseInfo.HoursAllocated = slot.AllocatedHours
			ds.Tasks = append(ds.Tasks, *baseInfo)
		}

		ds.TotalHours += slot.AllocatedHours
		dayAgg[dateKey] = ds
	}

	if len(dayAgg) == 0 {
		h.sendMessage(msg.Chat.ID, "🔎 Слотный просмотр: нет слотов с задачами (возможно, все задачи с нулевой длительностью).")
		return
	}

	// Преобразуем в слайс и сортируем по дате
	var daySchedules []models.DaySchedule
	for _, ds := range dayAgg {
		daySchedules = append(daySchedules, ds)
	}
	sort.Slice(daySchedules, func(i, j int) bool {
		return daySchedules[i].Date.Before(daySchedules[j].Date)
	})

	// Формируем ответ
	response := "🧪 Экспериментальное расписание по слотам (временные окна):\n\n"

	maxDays := 7
	for i, ds := range daySchedules {
		if i >= maxDays {
			response += fmt.Sprintf("\n... и ещё %d дней", len(daySchedules)-maxDays)
			break
		}
		response += formatDaySchedule(ds, user.DailyCapacity)
	}

	response += "\n❗️ Это предварительный просмотр. Для записи расписания в БД используйте обычную команду /schedule."

	h.sendMessage(msg.Chat.ID, response)
}

// handleToday handles /today command
func (h *BotHandler) handleToday(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "Ошибка получения пользователя")
		return
	}

	today := time.Now()
	if user.TimeZone != "" {
		if loc, err := time.LoadLocation(user.TimeZone); err == nil {
			today = today.In(loc)
		}
	}
	schedules, err := database.GetScheduleForDateRange(user.ID, today, today)
	if err != nil {
		log.Printf("Error getting schedule: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка получения расписания")
		return
	}

	if len(schedules) == 0 {
		h.sendMessage(msg.Chat.ID, "📭 На сегодня нет запланированных задач.\nПопробуйте команду /schedule, чтобы распланировать задачи.")
		return
	}

	response := "📅 Сегодня:\n\n"
	response += formatDaySchedule(schedules[0], user.DailyCapacity)

	h.sendMessage(msg.Chat.ID, response)
}

// handleWeek handles /week command
func (h *BotHandler) handleWeek(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "Ошибка получения пользователя")
		return
	}

	today := time.Now()
	if user.TimeZone != "" {
		if loc, err := time.LoadLocation(user.TimeZone); err == nil {
			today = today.In(loc)
		}
	}
	endDate := today.AddDate(0, 0, 7)

	schedules, err := database.GetScheduleForDateRange(user.ID, today, endDate)
	if err != nil {
		log.Printf("Error getting schedule: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка получения расписания")
		return
	}

	if len(schedules) == 0 {
		h.sendMessage(msg.Chat.ID, "📭 На эту неделю нет запланированных задач.\nПопробуйте команду /schedule, чтобы распланировать задачи.")
		return
	}

	response := "📅 Расписание на неделю:\n\n"
	for _, daySchedule := range schedules {
		response += formatDaySchedule(daySchedule, user.DailyCapacity)
	}

	h.sendMessage(msg.Chat.ID, response)
}

// handleComplete handles /complete command
func (h *BotHandler) handleComplete(msg *tgbotapi.Message) {
	args := msg.CommandArguments()
	if args == "" {
		h.sendMessage(msg.Chat.ID, "Укажите ID задачи: /complete [ID]")
		return
	}

	taskID, err := strconv.ParseInt(args, 10, 64)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "Неверный ID задачи")
		return
	}

	err = database.CompleteTask(taskID)
	if err != nil {
		log.Printf("Error completing task: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка при отметке задачи")
		return
	}

	h.sendMessage(msg.Chat.ID, "✅ Задача отмечена как выполненная!")
}

// handleDelete handles /delete command
func (h *BotHandler) handleDelete(msg *tgbotapi.Message) {
	args := msg.CommandArguments()
	if args == "" {
		h.sendMessage(msg.Chat.ID, "Укажите ID задачи: /delete [ID]")
		return
	}

	taskID, err := strconv.ParseInt(args, 10, 64)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "Неверный ID задачи")
		return
	}

	err = database.DeleteTask(taskID)
	if err != nil {
		log.Printf("Error deleting task: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка при удалении задачи")
		return
	}

	h.sendMessage(msg.Chat.ID, "🗑 Задача удалена")
}

// handleSettings handles /settings command
func (h *BotHandler) handleSettings(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "Ошибка получения пользователя")
		return
	}

	args := msg.CommandArguments()
	if args == "" {
		// Show current settings
		workDaysStr := formatWorkDays(user.WorkDays)
		if user.WorkStart == "" {
			user.WorkStart = "09:00"
		}
		if user.WorkEnd == "" {
			user.WorkEnd = "18:00"
		}
		response := fmt.Sprintf(`⚙️ Текущие настройки:

⏰ Часов в день: %.1f
📅 Рабочие дни: %s
🕒 Рабочее время: %s-%s
🌍 Таймзона: %s

Для изменения используйте:
/settings [часы] | [дни] | [HH:MM-HH:MM]
Примеры:
/settings 6 | 1,2,3,4,5
/settings 6 | 1,2,3,4,5 | 09:00-18:00`, user.DailyCapacity, workDaysStr, user.WorkStart, user.WorkEnd, user.TimeZone)

		h.sendMessage(msg.Chat.ID, response)
		return
	}

	// Parse new settings
	parts := strings.Split(args, "|")
	if len(parts) < 2 || len(parts) > 3 {
		h.sendMessage(msg.Chat.ID, "Формат: /settings [часы] | [дни] | [HH:MM-HH:MM]\nПример: /settings 6 | 1,2,3,4,5 | 09:00-18:00")
		return
	}

	hoursStr := strings.TrimSpace(parts[0])
	hours, err := strconv.ParseFloat(hoursStr, 64)
	if err != nil || hours <= 0 || hours > 24 {
		h.sendMessage(msg.Chat.ID, "Неверное количество часов (должно быть от 0 до 24)")
		return
	}

	daysStr := strings.TrimSpace(parts[1])
	daysParts := strings.Split(daysStr, ",")
	workDays := []int{}
	for _, dayStr := range daysParts {
		day, err := strconv.Atoi(strings.TrimSpace(dayStr))
		if err != nil || day < 1 || day > 7 {
			h.sendMessage(msg.Chat.ID, "Неверный день недели (1=Пн, 7=Вс)")
			return
		}
		workDays = append(workDays, day)
	}

	workStart := user.WorkStart
	workEnd := user.WorkEnd
	if workStart == "" {
		workStart = "09:00"
	}
	if workEnd == "" {
		workEnd = "18:00"
	}

	// Optional work hours part
	if len(parts) == 3 {
		workHoursStr := strings.TrimSpace(parts[2])
		segments := strings.Split(workHoursStr, "-")
		if len(segments) != 2 {
			h.sendMessage(msg.Chat.ID, "Неверный формат рабочего времени. Используйте HH:MM-HH:MM, например 09:00-18:00")
			return
		}

		startStr := strings.TrimSpace(segments[0])
		endStr := strings.TrimSpace(segments[1])

		startTime, err := time.Parse("15:04", startStr)
		if err != nil {
			h.sendMessage(msg.Chat.ID, "Неверный формат времени начала. Используйте HH:MM, например 09:00")
			return
		}
		endTime, err := time.Parse("15:04", endStr)
		if err != nil {
			h.sendMessage(msg.Chat.ID, "Неверный формат времени окончания. Используйте HH:MM, например 18:00")
			return
		}
		if !endTime.After(startTime) {
			h.sendMessage(msg.Chat.ID, "Время окончания должно быть позже времени начала.")
			return
		}

		workStart = startStr
		workEnd = endStr
	}

	err = database.UpdateUserSettings(user.ID, hours, workDays, workStart, workEnd)
	if err != nil {
		log.Printf("Error updating settings: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка при обновлении настроек")
		return
	}

	h.sendMessage(msg.Chat.ID, "✅ Настройки обновлены!")
}

// handleTimezone handles /timezone command
func (h *BotHandler) handleTimezone(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "Ошибка получения пользователя")
		return
	}

	args := strings.TrimSpace(msg.CommandArguments())
	if args == "" {
		h.sendMessage(msg.Chat.ID, fmt.Sprintf("🌍 Текущая таймзона: %s\n\nПример использования:\n/timezone Europe/Moscow", user.TimeZone))
		return
	}

	loc, err := time.LoadLocation(args)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "❗️ Не удалось распознать таймзону.\nИспользуйте имена из базы IANA, например: Europe/Moscow, Europe/Berlin, America/New_York.")
		return
	}

	_ = loc // только для проверки валидности

	if err := database.UpdateUserTimeZone(user.ID, args); err != nil {
		log.Printf("Error updating user timezone: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка при обновлении таймзоны")
		return
	}

	user.TimeZone = args
	h.sendMessage(msg.Chat.ID, fmt.Sprintf("✅ Таймзона обновлена: %s", user.TimeZone))
}

// handleGoogleConnect инициирует OAuth-флоу: бот выдаёт ссылку для авторизации в Google.
func (h *BotHandler) handleGoogleConnect(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "⚠️ Не удалось получить профиль пользователя.\nПопробуйте сначала выполнить команду /start.")
		return
	}

	cfg, err := googlecal.ConfigFromEnv()
	if err != nil {
		log.Printf("Error building Google OAuth config: %v", err)
		h.sendMessage(msg.Chat.ID, "⚠️ Интеграция с Google Calendar пока не настроена на сервере (отсутствуют GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET).")
		return
	}

	// В качестве state можно использовать ID пользователя, чтобы минимально защититься от CSRF.
	state := fmt.Sprintf("tguser-%d", user.ID)
	authURL := cfg.AuthCodeURL(
		state,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	text := fmt.Sprintf(`🔗 Подключение Google Calendar

1) Перейдите по ссылке ниже и войдите в свой Google‑аккаунт.
2) Разрешите доступ к календарю.
3) Скопируйте выданный код подтверждения.
4) Вернитесь в Telegram и отправьте команду:
<code>/google_code ВАШ_КОД</code>

Ссылка для авторизации:
%s`, authURL)

	h.sendMessage(msg.Chat.ID, text)
}

// handleGoogleCode принимает auth code от пользователя и сохраняет токены в БД.
func (h *BotHandler) handleGoogleCode(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "⚠️ Не удалось получить профиль пользователя.\nПопробуйте сначала выполнить команду /start.")
		return
	}

	code := strings.TrimSpace(msg.CommandArguments())
	if code == "" {
		h.sendMessage(msg.Chat.ID, "Отправьте код в формате:\n<code>/google_code ВАШ_КОД</code>")
		return
	}

	cfg, err := googlecal.ConfigFromEnv()
	if err != nil {
		log.Printf("Error building Google OAuth config: %v", err)
		h.sendMessage(msg.Chat.ID, "⚠️ Интеграция с Google Calendar пока не настроена на сервере (отсутствуют GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET).")
		return
	}

	ctx := context.Background()
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		log.Printf("Error exchanging Google auth code: %v", err)
		h.sendMessage(msg.Chat.ID, "❗️ Не удалось обменять код на токен.\nПроверьте, что вы используете свежий код и попробуйте ещё раз через /google_connect.")
		return
	}

	if err := database.SaveGoogleToken(user.ID, tok); err != nil {
		log.Printf("Error saving Google token: %v", err)
		h.sendMessage(msg.Chat.ID, "❗️ Не удалось сохранить токен Google.\nПопробуйте позже.")
		return
	}

	h.sendMessage(msg.Chat.ID, "✅ Google Calendar успешно подключен!\nТеперь при выполнении /schedule расписание будет выгружаться в ваш календарь.")
}

// handleGoogleStatus показывает, привязан ли Google Calendar к пользователю.
func (h *BotHandler) handleGoogleStatus(msg *tgbotapi.Message) {
	user, err := h.getUser(msg.From.ID)
	if err != nil {
		h.sendMessage(msg.Chat.ID, "⚠️ Не удалось получить профиль пользователя.\nПопробуйте сначала выполнить команду /start.")
		return
	}

	tok, err := database.GetGoogleToken(user.ID)
	if err != nil {
		log.Printf("Error getting Google token: %v", err)
		h.sendMessage(msg.Chat.ID, "Ошибка при получении статуса Google Calendar.")
		return
	}

	if tok == nil {
		h.sendMessage(msg.Chat.ID, "🔌 Google Calendar ещё не подключен.\nИспользуйте /google_connect, чтобы выдать доступ.")
		return
	}

	now := time.Now()
	status := "активен"
	if tok.Expiry.Before(now) {
		status = "истёк (будет автоматически обновлён при следующем экспорте, если есть refresh token)"
	}

	text := fmt.Sprintf(
		"✅ Google Calendar подключен.\nСостояние токена: %s\nСрок действия access token до: %s",
		status,
		tok.Expiry.Format("02.01.2006 15:04"),
	)

	h.sendMessage(msg.Chat.ID, text)
}

// Helper functions

func (h *BotHandler) getUser(telegramID int64) (*models.User, error) {
	return database.GetOrCreateUser(telegramID, "", "", "")
}

func (h *BotHandler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "HTML"

	if _, err := h.bot.Send(msg); err != nil {
		log.Printf("Error sending message: %v", err)
	}
}

func getStatusEmoji(status string) string {
	switch status {
	case "pending":
		return "⏳"
	case "scheduled":
		return "📅"
	case "in_progress":
		return "🔄"
	case "completed":
		return "✅"
	case "cancelled":
		return "❌"
	default:
		return "❓"
	}
}

func formatDaySchedule(daySchedule models.DaySchedule, dailyCapacity float64) string {
	weekday := getWeekdayRu(daySchedule.Date.Weekday())
	result := fmt.Sprintf("📆 %s, %s\n", weekday, daySchedule.Date.Format("02.01.2006"))
	result += fmt.Sprintf("⏱ Нагрузка: %.1f / %.1f ч\n", daySchedule.TotalHours, dailyCapacity)

	if dailyCapacity > 0 && daySchedule.TotalHours > dailyCapacity {
		result += "⚠️ День перегружен: запланировано больше, чем в настройках.\n"
	}

	result += "\n"

	for _, task := range daySchedule.Tasks {
		result += fmt.Sprintf("• %s (%.1f ч) ⭐️ %d\n", task.Title, task.HoursAllocated, task.Priority)
	}
	result += "\n"

	return result
}

func formatWorkDays(workDays []int) string {
	days := []string{}
	dayNames := map[int]string{
		1: "Пн", 2: "Вт", 3: "Ср", 4: "Чт", 5: "Пт", 6: "Сб", 7: "Вс",
	}
	for _, day := range workDays {
		days = append(days, dayNames[day])
	}
	return strings.Join(days, ", ")
}

func getWeekdayRu(weekday time.Weekday) string {
	days := map[time.Weekday]string{
		time.Monday:    "Понедельник",
		time.Tuesday:   "Вторник",
		time.Wednesday: "Среда",
		time.Thursday:  "Четверг",
		time.Friday:    "Пятница",
		time.Saturday:  "Суббота",
		time.Sunday:    "Воскресенье",
	}
	return days[weekday]
}

func parseDate(dateStr string) (time.Time, error) {
	// Try DD.MM.YYYY format
	t, err := time.Parse("02.01.2006", dateStr)
	if err == nil {
		return t, nil
	}

	// Try DD.MM.YY format
	t, err = time.Parse("02.01.06", dateStr)
	if err == nil {
		return t, nil
	}

	// Try YYYY-MM-DD format
	t, err = time.Parse("2006-01-02", dateStr)
	return t, err
}
