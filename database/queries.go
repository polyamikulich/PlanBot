package database

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/adkhorst/planbot/models"
	"github.com/lib/pq"
	"golang.org/x/oauth2"
)

// GetOrCreateUser gets existing user or creates a new one
func GetOrCreateUser(telegramID int64, username, firstName, lastName string) (*models.User, error) {
	user := &models.User{}

	// Try to get existing user
	query := `SELECT id, telegram_id, username, first_name, last_name, time_zone, work_start, work_end, daily_capacity, work_days, created_at, updated_at
			  FROM users WHERE telegram_id = $1`

	var workDays pq.Int64Array
	err := DB.QueryRow(query, telegramID).Scan(
		&user.ID,
		&user.TelegramID,
		&user.Username,
		&user.FirstName,
		&user.LastName,
		&user.TimeZone,
		&user.WorkStart,
		&user.WorkEnd,
		&user.DailyCapacity,
		&workDays,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		// Create new user
		insertQuery := `INSERT INTO users (telegram_id, username, first_name, last_name)
						VALUES ($1, $2, $3, $4)
						RETURNING id, telegram_id, username, first_name, last_name, time_zone, work_start, work_end, daily_capacity, work_days, created_at, updated_at`

		err = DB.QueryRow(insertQuery, telegramID, username, firstName, lastName).Scan(
			&user.ID,
			&user.TelegramID,
			&user.Username,
			&user.FirstName,
			&user.LastName,
			&user.TimeZone,
			&user.WorkStart,
			&user.WorkEnd,
			&user.DailyCapacity,
			&workDays,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to query user: %w", err)
	}

	// Convert pq.Int64Array to []int
	user.WorkDays = make([]int, len(workDays))
	for i, v := range workDays {
		user.WorkDays[i] = int(v)
	}

	return user, nil
}

// UpdateUserSettings updates user's daily capacity, work days and work hours
func UpdateUserSettings(userID int64, dailyCapacity float64, workDays []int, workStart, workEnd string) error {
	// Convert []int to pq.Int64Array
	workDaysArray := make(pq.Int64Array, len(workDays))
	for i, v := range workDays {
		workDaysArray[i] = int64(v)
	}

	query := `UPDATE users SET daily_capacity = $1, work_days = $2, work_start = $3, work_end = $4, updated_at = NOW()
			  WHERE id = $5`

	_, err := DB.Exec(query, dailyCapacity, workDaysArray, workStart, workEnd, userID)
	if err != nil {
		return fmt.Errorf("failed to update user settings: %w", err)
	}

	return nil
}

// UpdateUserTimeZone updates user's time zone
func UpdateUserTimeZone(userID int64, timeZone string) error {
	query := `UPDATE users SET time_zone = $1, updated_at = NOW()
			  WHERE id = $2`

	_, err := DB.Exec(query, timeZone, userID)
	if err != nil {
		return fmt.Errorf("failed to update user time zone: %w", err)
	}

	return nil
}

// CreateTask creates a new task
func CreateTask(task *models.Task) error {
	query := `INSERT INTO tasks (user_id, title, description, hours_required, priority, deadline)
			  VALUES ($1, $2, $3, $4, $5, $6)
			  RETURNING id, created_at, updated_at, status`

	err := DB.QueryRow(query,
		task.UserID,
		task.Title,
		task.Description,
		task.HoursRequired,
		task.Priority,
		task.Deadline,
	).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt, &task.Status)

	if err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	return nil
}

// GetUserTasks retrieves all tasks for a user
func GetUserTasks(userID int64) ([]models.Task, error) {
	query := `SELECT id, user_id, title, description, hours_required, priority, status, deadline, 
			  created_at, updated_at, completed_at
			  FROM tasks WHERE user_id = $1 ORDER BY priority DESC, deadline ASC NULLS LAST`

	rows, err := DB.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	tasks := []models.Task{}
	for rows.Next() {
		task := models.Task{}
		err := rows.Scan(
			&task.ID,
			&task.UserID,
			&task.Title,
			&task.Description,
			&task.HoursRequired,
			&task.Priority,
			&task.Status,
			&task.Deadline,
			&task.CreatedAt,
			&task.UpdatedAt,
			&task.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetPendingTasks retrieves all pending tasks for a user
func GetPendingTasks(userID int64) ([]models.Task, error) {
	query := `SELECT id, user_id, title, description, hours_required, priority, status, deadline, 
			  created_at, updated_at, completed_at
			  FROM tasks WHERE user_id = $1 AND status = 'pending' 
			  ORDER BY priority DESC, deadline ASC NULLS LAST`

	rows, err := DB.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending tasks: %w", err)
	}
	defer rows.Close()

	tasks := []models.Task{}
	for rows.Next() {
		task := models.Task{}
		err := rows.Scan(
			&task.ID,
			&task.UserID,
			&task.Title,
			&task.Description,
			&task.HoursRequired,
			&task.Priority,
			&task.Status,
			&task.Deadline,
			&task.CreatedAt,
			&task.UpdatedAt,
			&task.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetActiveTasks returns tasks that should participate in (re)planning.
// "Hard" rescheduling treats all non-completed / non-cancelled tasks as current.
func GetActiveTasks(userID int64) ([]models.Task, error) {
	query := `
		SELECT id, user_id, title, description, hours_required, priority, status, deadline,
			   created_at, updated_at, completed_at
		FROM tasks
		WHERE user_id = $1 AND status NOT IN ('completed', 'cancelled')
		ORDER BY priority DESC, deadline ASC NULLS LAST
	`

	rows, err := DB.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query active tasks: %w", err)
	}
	defer rows.Close()

	tasks := []models.Task{}
	for rows.Next() {
		task := models.Task{}
		err := rows.Scan(
			&task.ID,
			&task.UserID,
			&task.Title,
			&task.Description,
			&task.HoursRequired,
			&task.Priority,
			&task.Status,
			&task.Deadline,
			&task.CreatedAt,
			&task.UpdatedAt,
			&task.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan active task: %w", err)
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// UpdateTaskStatus updates the status of a task
func UpdateTaskStatus(taskID int64, status string) error {
	query := `UPDATE tasks SET status = $1, updated_at = NOW() WHERE id = $2`

	_, err := DB.Exec(query, status, taskID)
	if err != nil {
		return fmt.Errorf("failed to update task status: %w", err)
	}

	return nil
}

// CompleteTask marks a task as completed
func CompleteTask(taskID int64) error {
	query := `UPDATE tasks SET status = 'completed', completed_at = NOW(), updated_at = NOW() 
			  WHERE id = $1`

	_, err := DB.Exec(query, taskID)
	if err != nil {
		return fmt.Errorf("failed to complete task: %w", err)
	}

	return nil
}

// DeleteTask deletes a task
func DeleteTask(taskID int64) error {
	query := `DELETE FROM tasks WHERE id = $1`

	_, err := DB.Exec(query, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	return nil
}

// ClearTaskSchedules removes all schedules for given tasks
func ClearTaskSchedules(taskIDs []int64) error {
	if len(taskIDs) == 0 {
		return nil
	}

	query := `DELETE FROM task_schedules WHERE task_id = ANY($1)`
	_, err := DB.Exec(query, pq.Array(taskIDs))
	if err != nil {
		return fmt.Errorf("failed to clear task schedules: %w", err)
	}

	return nil
}

// SaveTaskSchedules saves schedule entries to database
func SaveTaskSchedules(schedules []models.DaySchedule) error {
	if len(schedules) == 0 {
		return nil
	}

	// Start transaction
	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare insert statement
	stmt, err := tx.Prepare(`INSERT INTO task_schedules (task_id, scheduled_date, hours_allocated)
							 VALUES ($1, $2, $3)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert all schedules
	taskIDs := make(map[int64]bool)
	for _, daySchedule := range schedules {
		for _, taskInfo := range daySchedule.Tasks {
			_, err := stmt.Exec(taskInfo.TaskID, daySchedule.Date, taskInfo.HoursAllocated)
			if err != nil {
				return fmt.Errorf("failed to insert schedule: %w", err)
			}
			taskIDs[taskInfo.TaskID] = true
		}
	}

	// Update task status to 'scheduled'
	for taskID := range taskIDs {
		_, err := tx.Exec(`UPDATE tasks SET status = 'scheduled', updated_at = NOW() WHERE id = $1`, taskID)
		if err != nil {
			return fmt.Errorf("failed to update task status: %w", err)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetScheduleForDateRange retrieves schedule for a date range
func GetScheduleForDateRange(userID int64, startDate, endDate time.Time) ([]models.DaySchedule, error) {
	query := `SELECT ts.scheduled_date, ts.task_id, t.title, ts.hours_allocated, t.priority, t.deadline
			  FROM task_schedules ts
			  JOIN tasks t ON ts.task_id = t.id
			  WHERE t.user_id = $1 AND ts.scheduled_date >= $2 AND ts.scheduled_date <= $3
			  ORDER BY ts.scheduled_date, t.priority DESC`

	rows, err := DB.Query(query, userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query schedules: %w", err)
	}
	defer rows.Close()

	// Group by date
	scheduleMap := make(map[string]*models.DaySchedule)

	for rows.Next() {
		var scheduledDate time.Time
		taskInfo := models.ScheduledTaskInfo{}

		err := rows.Scan(
			&scheduledDate,
			&taskInfo.TaskID,
			&taskInfo.Title,
			&taskInfo.HoursAllocated,
			&taskInfo.Priority,
			&taskInfo.Deadline,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan schedule: %w", err)
		}

		dateKey := scheduledDate.Format("2006-01-02")
		daySchedule, exists := scheduleMap[dateKey]
		if !exists {
			daySchedule = &models.DaySchedule{
				Date:       scheduledDate,
				Tasks:      []models.ScheduledTaskInfo{},
				TotalHours: 0,
			}
			scheduleMap[dateKey] = daySchedule
		}

		daySchedule.Tasks = append(daySchedule.Tasks, taskInfo)
		daySchedule.TotalHours += taskInfo.HoursAllocated
	}

	// Convert map to slice
	schedules := make([]models.DaySchedule, 0, len(scheduleMap))
	for _, schedule := range scheduleMap {
		schedules = append(schedules, *schedule)
	}

	return schedules, nil
}

// GetGoogleToken returns stored Google OAuth token for user.
func GetGoogleToken(userID int64) (*models.GoogleToken, error) {
	query := `SELECT user_id, access_token, refresh_token, expiry, created_at, updated_at
	          FROM user_google_tokens WHERE user_id = $1`

	tok := &models.GoogleToken{}
	err := DB.QueryRow(query, userID).Scan(
		&tok.UserID,
		&tok.AccessToken,
		&tok.RefreshToken,
		&tok.Expiry,
		&tok.CreatedAt,
		&tok.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get google token: %w", err)
	}
	return tok, nil
}

// SaveGoogleToken upserts user's Google OAuth token.
func SaveGoogleToken(userID int64, token *oauth2.Token) error {
	if token == nil {
		return fmt.Errorf("nil token")
	}
	query := `
		INSERT INTO user_google_tokens (user_id, access_token, refresh_token, expiry, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET access_token = EXCLUDED.access_token,
		    refresh_token = EXCLUDED.refresh_token,
		    expiry = EXCLUDED.expiry,
		    updated_at = NOW()
	`

	_, err := DB.Exec(query, userID, token.AccessToken, token.RefreshToken, token.Expiry)
	if err != nil {
		return fmt.Errorf("failed to save google token: %w", err)
	}
	return nil
}
