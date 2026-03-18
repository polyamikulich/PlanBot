-- PlanBot Database Migrations
-- Run this file to update existing database schema

-- Migration 1: Add scheduling fields to existing database
-- Run if upgrading from basic version

-- Add new columns to users table (if not exists)
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='users' AND column_name='daily_capacity') THEN
        ALTER TABLE users ADD COLUMN daily_capacity DECIMAL(5,2) DEFAULT 8.0;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='users' AND column_name='work_days') THEN
        ALTER TABLE users ADD COLUMN work_days INTEGER[] DEFAULT ARRAY[1,2,3,4,5];
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='users' AND column_name='time_zone') THEN
        ALTER TABLE users ADD COLUMN time_zone VARCHAR(255) DEFAULT 'Europe/Moscow';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='users' AND column_name='work_start') THEN
        ALTER TABLE users ADD COLUMN work_start VARCHAR(5) DEFAULT '09:00';
    END IF;

    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='users' AND column_name='work_end') THEN
        ALTER TABLE users ADD COLUMN work_end VARCHAR(5) DEFAULT '18:00';
    END IF;
END $$;

-- Add new columns to tasks table (if not exists)
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='tasks' AND column_name='hours_required') THEN
        ALTER TABLE tasks ADD COLUMN hours_required DECIMAL(5,2) NOT NULL DEFAULT 1.0;
    END IF;
    
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                   WHERE table_name='tasks' AND column_name='deadline') THEN
        ALTER TABLE tasks ADD COLUMN deadline TIMESTAMP;
    END IF;
    
    -- Rename due_date to deadline if it exists
    IF EXISTS (SELECT 1 FROM information_schema.columns 
               WHERE table_name='tasks' AND column_name='due_date') 
       AND NOT EXISTS (SELECT 1 FROM information_schema.columns 
                       WHERE table_name='tasks' AND column_name='deadline') THEN
        ALTER TABLE tasks RENAME COLUMN due_date TO deadline;
    END IF;
END $$;

-- Update task status values if needed
UPDATE tasks SET status = 'pending' WHERE status = 'open';
UPDATE tasks SET status = 'completed' WHERE status = 'done';

-- Create task_schedules table (if not exists)
CREATE TABLE IF NOT EXISTS task_schedules (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    scheduled_date DATE NOT NULL,
    hours_allocated DECIMAL(5,2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes if not exist
CREATE INDEX IF NOT EXISTS idx_tasks_deadline ON tasks(deadline);
CREATE INDEX IF NOT EXISTS idx_task_schedules_task_id ON task_schedules(task_id);
CREATE INDEX IF NOT EXISTS idx_task_schedules_date ON task_schedules(scheduled_date);

-- Drop old index if exists
DROP INDEX IF EXISTS idx_tasks_due_date;

COMMENT ON TABLE users IS 'Telegram users with their scheduling preferences';
COMMENT ON TABLE tasks IS 'User tasks with hours required and deadlines';
COMMENT ON TABLE task_schedules IS 'Scheduled task allocations by day';

-- Google OAuth tokens
CREATE TABLE IF NOT EXISTS user_google_tokens (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expiry TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);