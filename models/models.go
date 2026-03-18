package models

import (
	"time"
)

// User represents a Telegram user
type User struct {
	ID            int64
	TelegramID    int64
	Username      string
	FirstName     string
	LastName      string
	TimeZone      string  // e.g. "Europe/Moscow"
	WorkStart     string  // e.g. "09:00"
	WorkEnd       string  // e.g. "18:00"
	DailyCapacity float64 // hours per day
	WorkDays      []int   // 1=Monday, 7=Sunday
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type GoogleToken struct {
	UserID       int64
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Task represents a user's task
type Task struct {
	ID            int64
	UserID        int64
	Title         string
	Description   string
	HoursRequired float64
	Priority      int
	Status        string // pending, scheduled, in_progress, completed, cancelled
	Deadline      *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	CompletedAt   *time.Time
}

// TaskSchedule represents when a task is scheduled
type TaskSchedule struct {
	ID             int64
	TaskID         int64
	ScheduledDate  time.Time
	HoursAllocated float64
	CreatedAt      time.Time
}

// DaySchedule represents all tasks scheduled for a specific day
type DaySchedule struct {
	Date           time.Time
	Tasks          []ScheduledTaskInfo
	TotalHours     float64
	AvailableHours float64
}

// ScheduledTaskInfo contains task details with scheduling info
type ScheduledTaskInfo struct {
	TaskID         int64
	Title          string
	HoursAllocated float64
	Priority       int
	Deadline       *time.Time
}

// ScheduleRequest represents a request to schedule tasks
type ScheduleRequest struct {
	UserID    int64
	StartDate time.Time
}

// ScheduleResult represents the result of scheduling
type ScheduleResult struct {
	Success          bool
	Message          string
	DaySchedules     []DaySchedule
	UnscheduledTasks []int64 // IDs of tasks that couldn't be scheduled
}

// TimeSlot represents a concrete time interval inside a day
// used for future fine-grained scheduling (by time of day).
type TimeSlot struct {
	UserID         int64
	Date           time.Time // calendar date in user's time zone
	Start          time.Time // exact start time
	End            time.Time // exact end time
	CapacityHours  float64   // total capacity of this slot (usually 1.0 for 60 minutes)
	AllocatedHours float64   // how many hours are already allocated
	TaskID         *int64    // optional: ID of the task occupying this slot
	Source         string    // e.g. "task", "external", "blocked", ""
}
