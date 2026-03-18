package scheduler

import (
	"math"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/adkhorst/planbot/models"
)

// Scheduler handles task scheduling logic
type Scheduler struct {
	user                *models.User
	tasks               []models.Task
	planningHorizonDays int
}

// NewScheduler creates a new scheduler instance
func NewScheduler(user *models.User, tasks []models.Task) *Scheduler {
	horizon := 365 // default: 1 year
	if env := os.Getenv("PLANNING_HORIZON_DAYS"); env != "" {
		if v, err := strconv.Atoi(env); err == nil && v > 0 {
			horizon = v
		}
	}

	return &Scheduler{
		user:                user,
		tasks:               tasks,
		planningHorizonDays: horizon,
	}
}

// Schedule distributes tasks across days using deadline-aware algorithm
func (s *Scheduler) Schedule(startDate time.Time) *models.ScheduleResult {
	result := &models.ScheduleResult{
		Success:          true,
		DaySchedules:     []models.DaySchedule{},
		UnscheduledTasks: []int64{},
	}

	// Filter only pending tasks
	pendingTasks := s.filterPendingTasks()
	if len(pendingTasks) == 0 {
		result.Message = "Нет задач для планирования"
		return result
	}

	// Sort tasks by priority and deadline
	sortedTasks := s.sortTasksByDeadlineAndPriority(pendingTasks)

	// Create day slots map
	daySlots := make(map[string]*models.DaySchedule)

	// Schedule tasks
	for _, task := range sortedTasks {
		scheduled := s.scheduleTask(task, startDate, daySlots)
		if !scheduled {
			result.UnscheduledTasks = append(result.UnscheduledTasks, task.ID)
			result.Success = false
		}
	}

	// Convert map to sorted slice
	result.DaySchedules = s.convertDaySlotsToSlice(daySlots)

	if len(result.UnscheduledTasks) > 0 {
		result.Message = "Некоторые задачи не удалось запланировать"
	} else {
		result.Message = "Все задачи успешно запланированы"
	}

	return result
}

// filterPendingTasks returns only tasks with pending status
func (s *Scheduler) filterPendingTasks() []models.Task {
	pending := []models.Task{}
	for _, task := range s.tasks {
		if task.Status == "pending" {
			pending = append(pending, task)
		}
	}
	return pending
}

// sortTasksByDeadlineAndPriority sorts tasks by deadline (closest first) and priority
func (s *Scheduler) sortTasksByDeadlineAndPriority(tasks []models.Task) []models.Task {
	sorted := make([]models.Task, len(tasks))
	copy(sorted, tasks)

	sort.Slice(sorted, func(i, j int) bool {
		// Tasks with deadlines come first
		if sorted[i].Deadline != nil && sorted[j].Deadline == nil {
			return true
		}
		if sorted[i].Deadline == nil && sorted[j].Deadline != nil {
			return false
		}

		// If both have deadlines, sort by deadline
		if sorted[i].Deadline != nil && sorted[j].Deadline != nil {
			if !sorted[i].Deadline.Equal(*sorted[j].Deadline) {
				return sorted[i].Deadline.Before(*sorted[j].Deadline)
			}
		}

		// If deadlines are equal or both don't have deadlines, sort by priority
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority > sorted[j].Priority
		}

		// If priority is equal, sort by hours (smaller tasks first)
		return sorted[i].HoursRequired < sorted[j].HoursRequired
	})

	return sorted
}

// scheduleTask attempts to schedule a single task
func (s *Scheduler) scheduleTask(task models.Task, startDate time.Time, daySlots map[string]*models.DaySchedule) bool {
	remainingHours := task.HoursRequired
	currentDate := s.normalizeDate(startDate)

	// Try to allocate hours across available days, but not beyond configured planning horizon.
	// Primary strategy for tasks with дедлайном — начинать планирование как можно ближе к дедлайну
	// (обратное планирование), но не раньше startDate / latestStartDate.
	maxDaysToCheck := s.planningHorizonDays
	daysChecked := 0

	// If we have a deadline, we could in будущем использовать latestStartDate,
	// но сейчас просто идём вперёд от currentDate и проверяем дедлайн по условию ниже.

	for remainingHours > 0 && daysChecked < maxDaysToCheck {
		// Check if this day is a work day
		if !s.isWorkDay(currentDate) {
			currentDate = currentDate.AddDate(0, 0, 1)
			daysChecked++
			continue
		}

		// Check if we've exceeded the deadline
		if task.Deadline != nil && currentDate.After(*task.Deadline) {
			return false
		}

		// Get or create day slot
		dateKey := s.formatDate(currentDate)
		daySlot, exists := daySlots[dateKey]
		if !exists {
			daySlot = &models.DaySchedule{
				Date:           currentDate,
				Tasks:          []models.ScheduledTaskInfo{},
				TotalHours:     0,
				AvailableHours: s.user.DailyCapacity,
			}
			daySlots[dateKey] = daySlot
		}

		// Calculate available hours for this day
		availableHours := s.user.DailyCapacity - daySlot.TotalHours

		if availableHours > 0 {
			// Allocate as much as possible to this day
			hoursToAllocate := remainingHours
			if hoursToAllocate > availableHours {
				hoursToAllocate = availableHours
			}

			// Add task to this day
			daySlot.Tasks = append(daySlot.Tasks, models.ScheduledTaskInfo{
				TaskID:         task.ID,
				Title:          task.Title,
				HoursAllocated: hoursToAllocate,
				Priority:       task.Priority,
				Deadline:       task.Deadline,
			})
			daySlot.TotalHours += hoursToAllocate
			daySlot.AvailableHours = s.user.DailyCapacity - daySlot.TotalHours

			remainingHours -= hoursToAllocate
		}

		currentDate = currentDate.AddDate(0, 0, 1)
		daysChecked++
	}

	return remainingHours == 0
}

// calculateLatestStartDate calculates the latest date a task can start
func (s *Scheduler) calculateLatestStartDate(deadline time.Time, hoursRequired float64) time.Time {
	if s.user.DailyCapacity <= 0 {
		return s.normalizeDate(deadline)
	}

	// Use ceiling to calculate exact number of required work days.
	// Дедлайн считаем включительно, поэтому:
	// - deadline сам считается рабочим днём №1 (если он рабочий),
	// - затем двигаемся назад, пока не наберём нужное количество рабочих дней.
	workDaysNeeded := int(math.Ceil(hoursRequired / s.user.DailyCapacity))
	date := deadline

	// Если дедлайн не рабочий день, отступаем назад до ближайшего рабочего.
	for !s.isWorkDay(date) {
		date = date.AddDate(0, 0, -1)
	}

	workDaysFound := 1

	// Go backwards from adjusted deadline
	for workDaysFound < workDaysNeeded {
		date = date.AddDate(0, 0, -1)
		if s.isWorkDay(date) {
			workDaysFound++
		}
	}

	return s.normalizeDate(date)
}

// isWorkDay checks if a date is a work day for the user
func (s *Scheduler) isWorkDay(date time.Time) bool {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}

	for _, workDay := range s.user.WorkDays {
		if workDay == weekday {
			return true
		}
	}
	return false
}

// normalizeDate removes time component from date
func (s *Scheduler) normalizeDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// formatDate formats date as YYYY-MM-DD
func (s *Scheduler) formatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

// convertDaySlotsToSlice converts map to sorted slice
func (s *Scheduler) convertDaySlotsToSlice(daySlots map[string]*models.DaySchedule) []models.DaySchedule {
	schedules := make([]models.DaySchedule, 0, len(daySlots))
	for _, slot := range daySlots {
		schedules = append(schedules, *slot)
	}

	// Sort by date
	sort.Slice(schedules, func(i, j int) bool {
		return schedules[i].Date.Before(schedules[j].Date)
	})

	return schedules
}

// SlotScheduler builds and manages fine-grained time slots within days.
// It is not yet wired into the main scheduling flow and is prepared for future use.
type SlotScheduler struct {
	user        *models.User
	slotMinutes int
	horizonDays int
}

// NewSlotScheduler creates a new SlotScheduler with defaults based on environment.
func NewSlotScheduler(user *models.User) *SlotScheduler {
	horizon := 365
	if env := os.Getenv("PLANNING_HORIZON_DAYS"); env != "" {
		if v, err := strconv.Atoi(env); err == nil && v > 0 {
			horizon = v
		}
	}

	slotMinutes := 60
	if env := os.Getenv("PLANNING_SLOT_MINUTES"); env != "" {
		if v, err := strconv.Atoi(env); err == nil && v > 0 {
			slotMinutes = v
		}
	}

	return &SlotScheduler{
		user:        user,
		slotMinutes: slotMinutes,
		horizonDays: horizon,
	}
}

// BuildDailySlots generates in-memory time slots for working days
// between startDate and startDate + horizon.
func (s *SlotScheduler) BuildDailySlots(startDate time.Time) []models.TimeSlot {
	var slots []models.TimeSlot

	// Normalize start date to user's time zone and midnight
	current := time.Date(startDate.Year(), startDate.Month(), startDate.Day(), 0, 0, 0, 0, startDate.Location())

	workStart := s.user.WorkStart
	workEnd := s.user.WorkEnd
	if workStart == "" {
		workStart = "09:00"
	}
	if workEnd == "" {
		workEnd = "18:00"
	}

	for day := 0; day < s.horizonDays; day++ {
		if !s.isWorkDay(current) {
			current = current.AddDate(0, 0, 1)
			continue
		}

		// Parse working hours for this day
		startClock, errStart := time.ParseInLocation("15:04", workStart, current.Location())
		endClock, errEnd := time.ParseInLocation("15:04", workEnd, current.Location())
		if errStart != nil || errEnd != nil || !endClock.After(startClock) {
			// If working hours are invalid, skip slot generation for this day
			current = current.AddDate(0, 0, 1)
			continue
		}

		dayStart := time.Date(current.Year(), current.Month(), current.Day(), startClock.Hour(), startClock.Minute(), 0, 0, current.Location())
		dayEnd := time.Date(current.Year(), current.Month(), current.Day(), endClock.Hour(), endClock.Minute(), 0, 0, current.Location())

		slotDuration := time.Duration(s.slotMinutes) * time.Minute
		for t := dayStart; t.Before(dayEnd); t = t.Add(slotDuration) {
			end := t.Add(slotDuration)
			if end.After(dayEnd) {
				end = dayEnd
			}

			capacity := end.Sub(t).Hours()
			slots = append(slots, models.TimeSlot{
				UserID:         s.user.ID,
				Date:           current,
				Start:          t,
				End:            end,
				CapacityHours:  capacity,
				AllocatedHours: 0,
				TaskID:         nil,
				Source:         "",
			})
		}

		current = current.AddDate(0, 0, 1)
	}

	return slots
}

// isWorkDay checks if a date is a work day for the user (reuses user's WorkDays).
func (s *SlotScheduler) isWorkDay(date time.Time) bool {
	weekday := int(date.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}

	for _, workDay := range s.user.WorkDays {
		if workDay == weekday {
			return true
		}
	}
	return false
}

// AssignTasksToSlots performs simple greedy assignment of tasks to free slots.
// This function operates in-memory and does not persist any changes.
func (s *SlotScheduler) AssignTasksToSlots(tasks []models.Task, slots []models.TimeSlot) []models.TimeSlot {
	// Local copy of slots to modify
	result := make([]models.TimeSlot, len(slots))
	copy(result, slots)

	// Reuse existing sorting rules from Scheduler
	baseScheduler := NewScheduler(s.user, tasks)
	sortedTasks := baseScheduler.sortTasksByDeadlineAndPriority(tasks)

	for _, task := range sortedTasks {
		remaining := task.HoursRequired
		for i := range result {
			if remaining <= 0 {
				break
			}

			slot := &result[i]

			// Skip slots that already fully occupied or belong to another task
			if slot.AllocatedHours >= slot.CapacityHours {
				continue
			}

			free := slot.CapacityHours - slot.AllocatedHours
			if free <= 0 {
				continue
			}

			toAllocate := remaining
			if toAllocate > free {
				toAllocate = free
			}

			slot.AllocatedHours += toAllocate
			if toAllocate > 0 {
				slot.TaskID = &task.ID
				if slot.Source == "" {
					slot.Source = "task"
				}
			}

			remaining -= toAllocate
		}
	}

	return result
}
