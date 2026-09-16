package mail

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DatabaseLedger keeps deliveries in the mail_deliveries table.
type DatabaseLedger struct{ pool *pgxpool.Pool }

func NewDatabaseLedger(pool *pgxpool.Pool) *DatabaseLedger { return &DatabaseLedger{pool: pool} }

func (l *DatabaseLedger) Record(ctx context.Context, delivery Delivery) error {
	var presentationID any
	if strings.TrimSpace(delivery.PresentationID) != "" {
		presentationID = delivery.PresentationID
	}
	var actorID any
	if strings.TrimSpace(delivery.ActorID) != "" {
		actorID = delivery.ActorID
	}
	_, err := l.pool.Exec(ctx, `INSERT INTO mail_deliveries(id,event,recipient,subject,presentation_id,actor_id,status,attempts,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,0,$8,$8)`,
		delivery.ID, delivery.Event, delivery.Recipient, delivery.Subject, presentationID, actorID, StatusQueued, delivery.CreatedAt)
	return err
}

func (l *DatabaseLedger) Complete(ctx context.Context, id, status string, attempts int, errorMessage string, at time.Time) error {
	_, err := l.pool.Exec(ctx, `UPDATE mail_deliveries SET status=$2,attempts=GREATEST(attempts,$3),error_message=$4,updated_at=$5 WHERE id=$1`,
		id, status, attempts, errorMessage, at)
	return err
}

func (l *DatabaseLedger) List(ctx context.Context, status string, limit int) (Page, error) {
	page := Page{Items: []Delivery{}, Status: map[string]int{}}
	query := `SELECT id::text,event,recipient,subject,coalesce(presentation_id::text,''),coalesce(actor_id::text,''),status,attempts,error_message,created_at,updated_at
		FROM mail_deliveries`
	args := []any{limit}
	if status != "" {
		query += ` WHERE status=$2`
		args = append(args, status)
	}
	rows, err := l.pool.Query(ctx, query+` ORDER BY created_at DESC, id LIMIT $1`, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Delivery
		if err := rows.Scan(&item.ID, &item.Event, &item.Recipient, &item.Subject, &item.PresentationID, &item.ActorID,
			&item.Status, &item.Attempts, &item.ErrorMessage, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return Page{}, err
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	counts, err := l.pool.Query(ctx, `SELECT status, count(*) FROM mail_deliveries GROUP BY 1`)
	if err != nil {
		return Page{}, err
	}
	defer counts.Close()
	for counts.Next() {
		var key string
		var count int
		if err := counts.Scan(&key, &count); err != nil {
			return Page{}, err
		}
		page.Status[key] = count
		page.Total += count
	}
	return page, counts.Err()
}
