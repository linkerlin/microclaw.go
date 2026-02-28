package tools

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/linkerlin/microclaw.go/internal/db"
)

// AddScheduleInput is the input for add_schedule.
type AddScheduleInput struct {
	Name        string `json:"name" jsonschema_description:"Name of the scheduled task"`
	Description string `json:"description,omitempty" jsonschema_description:"What the task should do when it runs"`
	CronExpr    string `json:"cron_expr,omitempty" jsonschema_description:"Cron expression for recurring tasks (e.g., '0 9 * * *' for 9am daily)"`
	OneTimeAt   string `json:"one_time_at,omitempty" jsonschema_description:"ISO8601 datetime for one-time task (e.g., '2024-01-01T09:00:00Z')"`
}

// AddScheduleOutput is the output for add_schedule.
type AddScheduleOutput struct {
	Success bool   `json:"success"`
	TaskID  int64  `json:"task_id"`
	Message string `json:"message"`
}

// ListSchedulesInput is the input for list_schedules.
type ListSchedulesInput struct{}

// ListSchedulesOutput is the output for list_schedules.
type ListSchedulesOutput struct {
	Tasks []ScheduleInfo `json:"tasks"`
	Count int            `json:"count"`
}

// ScheduleInfo describes a scheduled task.
type ScheduleInfo struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	CronExpr    string `json:"cron_expr"`
	Status      string `json:"status"`
	RunCount    int    `json:"run_count"`
}

// DeleteScheduleInput is the input for delete_schedule.
type DeleteScheduleInput struct {
	TaskID int64 `json:"task_id" jsonschema_description:"ID of the task to delete"`
}

// DeleteScheduleOutput is the output for delete_schedule.
type DeleteScheduleOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// NewAddScheduleTool creates the add_schedule tool.
func NewAddScheduleTool(database *db.DB, chatID int64, channel string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "add_schedule",
		Description: "Schedule a recurring or one-time task.",
	}, func(_ tool.Context, input AddScheduleInput) (AddScheduleOutput, error) {
		if input.CronExpr == "" && input.OneTimeAt == "" {
			return AddScheduleOutput{Success: false, Message: "either cron_expr or one_time_at is required"}, nil
		}

		task := &db.ScheduledTask{
			ChatID:      chatID,
			ChatChannel: channel,
			Name:        input.Name,
			Description: input.Description,
			CronExpr:    input.CronExpr,
		}

		if input.CronExpr != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			sched, err := parser.Parse(input.CronExpr)
			if err != nil {
				return AddScheduleOutput{Success: false, Message: fmt.Sprintf("invalid cron expression: %v", err)}, nil
			}
			next := sched.Next(time.Now())
			task.NextRun = &next
		}

		if input.OneTimeAt != "" {
			t, err := time.Parse(time.RFC3339, input.OneTimeAt)
			if err != nil {
				// Try other formats.
				t, err = time.Parse("2006-01-02T15:04:05", input.OneTimeAt)
				if err != nil {
					return AddScheduleOutput{Success: false, Message: fmt.Sprintf("invalid one_time_at: %v", err)}, nil
				}
			}
			task.OneTimeAt = &t
			task.NextRun = &t
		}

		id, err := database.SaveScheduledTask(task)
		if err != nil {
			return AddScheduleOutput{Success: false, Message: err.Error()}, nil
		}

		return AddScheduleOutput{
			Success: true,
			TaskID:  id,
			Message: fmt.Sprintf("task %q scheduled with ID %d", input.Name, id),
		}, nil
	})
}

// NewListSchedulesTool creates the list_schedules tool.
func NewListSchedulesTool(database *db.DB, chatID int64, channel string) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "list_schedules",
		Description: "List all scheduled tasks for this chat.",
	}, func(_ tool.Context, _ ListSchedulesInput) (ListSchedulesOutput, error) {
		tasks, err := database.GetScheduledTasks(chatID, channel)
		if err != nil {
			return ListSchedulesOutput{}, err
		}
		var infos []ScheduleInfo
		for _, t := range tasks {
			infos = append(infos, ScheduleInfo{
				ID:          t.ID,
				Name:        t.Name,
				Description: t.Description,
				CronExpr:    t.CronExpr,
				Status:      t.Status,
				RunCount:    t.RunCount,
			})
		}
		if infos == nil {
			infos = []ScheduleInfo{}
		}
		return ListSchedulesOutput{Tasks: infos, Count: len(infos)}, nil
	})
}

// NewDeleteScheduleTool creates the delete_schedule tool.
func NewDeleteScheduleTool(database *db.DB) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "delete_schedule",
		Description: "Delete a scheduled task by its ID.",
	}, func(_ tool.Context, input DeleteScheduleInput) (DeleteScheduleOutput, error) {
		if err := database.DeleteScheduledTask(input.TaskID); err != nil {
			return DeleteScheduleOutput{Success: false, Message: err.Error()}, nil
		}
		return DeleteScheduleOutput{
			Success: true,
			Message: fmt.Sprintf("task %d deleted", input.TaskID),
		}, nil
	})
}

// FormatScheduleList formats schedule list for human display.
func FormatScheduleList(tasks []*db.ScheduledTask) string {
	if len(tasks) == 0 {
		return "No scheduled tasks."
	}
	var sb strings.Builder
	for _, t := range tasks {
		schedule := t.CronExpr
		if schedule == "" && t.OneTimeAt != nil {
			schedule = "one-time at " + t.OneTimeAt.Format(time.RFC3339)
		}
		fmt.Fprintf(&sb, "- [%d] %s (%s) runs:%d\n", t.ID, t.Name, schedule, t.RunCount)
	}
	return sb.String()
}
