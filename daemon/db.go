package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"org.thymewatch.daemon/ctxlog"
)

func createDb(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS alarms (
		id REAL PRIMARY KEY,
		expiry_time TEXT NOT NULL,
		name TEXT, -- NULLABLE
		started_at TEXT NOT NULL,
		original_expiry_time TEXT NOT NULL,
		paused_at_time TEXT, -- NULLABLE
		is_expired INTEGER NOT NULL,
		is_dismissed INTEGER NOT NULL
	) STRICT;
	`)
	if err != nil {
		return err
	}
	return nil
}

func serializeTimeForDb(t time.Time) string {
	return t.Format(time.RFC3339Nano)
}

func deserializeTimeFromDb(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func insertAlarm(ctx context.Context, db *sql.DB, alarm *StoredAlarm) error {
	stmt, err := db.PrepareContext(ctx, `
	INSERT INTO alarms (id, expiry_time, name, started_at,
		original_expiry_time, paused_at_time, is_expired, is_dismissed)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.ExecContext(ctx, alarm.Id, serializeTimeForDb(alarm.Spec.ExpiryTime),
		alarm.Spec.Name.ToNillable(), serializeTimeForDb(alarm.Spec.StartedAt),
		serializeTimeForDb(alarm.OriginalExpiryTime),
		alarm.PausedAtTime.Map(serializeTimeForDb).ToNillable(),
		alarm.IsExpired, alarm.IsDismissed)
	return err
}

type UpdateSpec struct {
	ColumnName string
	Value      any
}

func updateAlarms(ctx context.Context, db *sql.DB, ids []float64, updates ...UpdateSpec) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	txIsActive := true
	defer func() {
		if txIsActive {
			tx.Rollback()
		}
	}()
	updateStr := strings.Builder{}
	args := make([]any, len(updates)+1)
	for i, update := range updates {
		if i > 0 {
			updateStr.WriteString(", ")
		}
		updateStr.WriteString(update.ColumnName)
		updateStr.WriteString(" = $")
		updateStr.WriteString(strconv.Itoa((i + 1)))
		args[i] = update.Value
	}
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
	UPDATE alarms SET %v
	WHERE id = $%v
	`, updateStr.String(), len(updates)+1))
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, id := range ids {
		args[len(args)-1] = id
		_, err = stmt.ExecContext(ctx, args...)
		if err != nil {
			return err
		}
	}
	stmt.Close()
	err = tx.Commit()
	if err != nil {
		return err
	}
	txIsActive = false
	return nil
}

func deleteAlarm(ctx context.Context, db *sql.DB, id float64) error {
	stmt, err := db.PrepareContext(ctx, "DELETE FROM alarms WHERE id = $1")
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = stmt.ExecContext(ctx, id)
	return err
}

func ParseDbTime(ctx context.Context, str string, fieldName string, id float64,
	dest *time.Time,
) bool {
	time, err := deserializeTimeFromDb(str)
	if err != nil {
		ctxlog.Printf(ctx, "Could not parse paused_at_time field for alarm with id=%v: %v",
			id, err)
		return false
	}
	*dest = time
	return true
}

func getAlarms(ctx context.Context, db *sql.DB) (map[float64]*StoredAlarm, error) {
	rows, err := db.QueryContext(ctx, `
	SELECT id, expiry_time, name, started_at, original_expiry_time,
		paused_at_time, is_expired, is_dismissed
	FROM alarms`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	alarmsMap := make(map[float64]*StoredAlarm)
	for rows.Next() {
		alarm := &StoredAlarm{}
		var name sql.NullString
		var pausedAtTime sql.NullString
		var expiryTime string
		var startedAtTime string
		var originalExpiryTime string
		err := rows.Scan(&alarm.Id, &expiryTime, &name,
			&startedAtTime, &originalExpiryTime, &pausedAtTime,
			&alarm.IsExpired, &alarm.IsDismissed)
		if err != nil {
			return nil, err
		}
		if name.Valid {
			alarm.Spec.Name = Some(name.String)
		}
		if pausedAtTime.Valid {
			var t time.Time
			ok := ParseDbTime(ctx, pausedAtTime.String, "paused_at_time",
				alarm.Id, &t)
			if !ok {
				continue
			}
			alarm.PausedAtTime = Some(t)
		}
		ok := ParseDbTime(ctx, expiryTime, "expiry_time", alarm.Id, &alarm.Spec.ExpiryTime)
		if !ok {
			continue
		}
		ok = ParseDbTime(ctx, startedAtTime, "started_at_time", alarm.Id,
			&alarm.Spec.StartedAt)
		if !ok {
			continue
		}
		ok = ParseDbTime(ctx, originalExpiryTime, "original_expiry_time",
			alarm.Id, &alarm.OriginalExpiryTime)
		if !ok {
			continue
		}
		alarmsMap[alarm.Id] = alarm
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return alarmsMap, nil
}
