package googlecal

import (
	"context"
	"fmt"
	"os"

	"github.com/adkhorst/planbot/models"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

var calendarScopes = []string{
	calendar.CalendarScope,
}

func ConfigFromEnv() (*oauth2.Config, error) {
	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET not set")
	}
	return &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       calendarScopes,
		Endpoint:     google.Endpoint,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob", // для desktop/CLI-флоу
	}, nil
}

// Client is a thin wrapper around Google Calendar service.
// В текущей простой версии он использует уже полученный access token,
// переданный извне (без refresh токена и полноценного OAuth‑флоу).
type Client struct {
	svc *calendar.Service
}

// NewFromAccessToken создает клиента Google Calendar, используя "сырое" значение access token.
// Предполагается, что токен уже получен другим способом и еще не истек.
func NewFromAccessToken(ctx context.Context, accessToken string) (*Client, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("googlecal: empty access token")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: accessToken,
	})

	svc, err := calendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("googlecal: create calendar service: %w", err)
	}

	return &Client{svc: svc}, nil
}

// ExportDaySchedules выполняет самый простой экспорт:
// для каждой задачи в каждом дне создает событие "на весь день" в указанном календаре.
// Временные слоты внутри дня пока не используются.
func (c *Client) ExportDaySchedules(ctx context.Context, calendarID string, user *models.User, schedules []models.DaySchedule) error {
	if calendarID == "" {
		calendarID = "primary"
	}

	for _, day := range schedules {
		// Определяем дату в формате YYYY-MM-DD
		date := day.Date.Format("2006-01-02")

		// Событие-allDay в Google Calendar задается как [start.date, end.date),
		// поэтому конец = следующий день.
		endDate := day.Date.AddDate(0, 0, 1).Format("2006-01-02")

		for _, task := range day.Tasks {
			summary := task.Title
			if summary == "" {
				summary = fmt.Sprintf("Задача #%d", task.TaskID)
			}

			description := fmt.Sprintf("Планируемые часы: %.1f\nПриоритет: %d", task.HoursAllocated, task.Priority)
			if task.Deadline != nil {
				description += fmt.Sprintf("\nДедлайн: %s", task.Deadline.Format("02.01.2006"))
			}

			ev := &calendar.Event{
				Summary:     summary,
				Description: description,
				Start: &calendar.EventDateTime{
					Date: date,
				},
				End: &calendar.EventDateTime{
					Date: endDate,
				},
			}

			_, err := c.svc.Events.Insert(calendarID, ev).Context(ctx).Do()
			if err != nil {
				return fmt.Errorf("googlecal: insert event for %s on %s: %w", summary, date, err)
			}
		}
	}

	return nil
}

// NewWithStoredToken создает клиента на основе oauth2.Config и сохранённого токена.
// При необходимости обновляет access_token через refresh_token.
func NewWithStoredToken(ctx context.Context, cfg *oauth2.Config, stored *models.GoogleToken, save func(*oauth2.Token) error) (*Client, error) {
	if cfg == nil || stored == nil {
		return nil, fmt.Errorf("missing config or token")
	}

	tok := &oauth2.Token{
		AccessToken:  stored.AccessToken,
		RefreshToken: stored.RefreshToken,
		Expiry:       stored.Expiry,
	}

	ts := cfg.TokenSource(ctx, tok)

	// Получаем актуальный токен (мог обновиться).
	newTok, err := ts.Token()
	if err != nil {
		return nil, fmt.Errorf("googlecal: refresh token: %w", err)
	}

	// Если изменился access/expiry — сохранить через коллбек.
	if save != nil && (newTok.AccessToken != tok.AccessToken || !newTok.Expiry.Equal(tok.Expiry)) {
		if err := save(newTok); err != nil {
			return nil, fmt.Errorf("googlecal: save refreshed token: %w", err)
		}
	}

	svc, err := calendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("googlecal: create calendar service: %w", err)
	}

	return &Client{svc: svc}, nil
}
