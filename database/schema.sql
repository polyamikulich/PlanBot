-- Users table
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT UNIQUE NOT NULL,
    username VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    time_zone VARCHAR(255) DEFAULT 'Europe/Moscow',
    work_start VARCHAR(5) DEFAULT '09:00',
    work_end VARCHAR(5) DEFAULT '18:00',
    daily_capacity DECIMAL(5,2) DEFAULT 8.0, -- hours per day
    work_days INTEGER[] DEFAULT ARRAY[1,2,3,4,5], -- 1=Monday, 7=Sunday
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Tasks table
CREATE TABLE IF NOT EXISTS tasks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    hours_required DECIMAL(5,2) NOT NULL, -- hours needed to complete
    priority INTEGER DEFAULT 0, -- higher = more important
    status VARCHAR(50) DEFAULT 'pending', -- pending, scheduled, in_progress, completed, cancelled
    deadline TIMESTAMP, -- hard deadline
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP
);

-- Task schedules table (tracks when tasks are scheduled)
CREATE TABLE IF NOT EXISTS task_schedules (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    scheduled_date DATE NOT NULL, -- which day
    hours_allocated DECIMAL(5,2) NOT NULL, -- how many hours on this day
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for better performance
CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);
CREATE INDEX IF NOT EXISTS idx_tasks_user_id ON tasks(user_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_deadline ON tasks(deadline);
CREATE INDEX IF NOT EXISTS idx_task_schedules_task_id ON task_schedules(task_id);
CREATE INDEX IF NOT EXISTS idx_task_schedules_date ON task_schedules(scheduled_date);