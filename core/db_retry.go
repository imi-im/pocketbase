package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
)

// default retries intervals (in ms)
var defaultRetryIntervals = []int{50, 100, 150, 200, 300, 400, 500, 700, 1000}

var retryableSQLStates = map[string]struct{}{
	"55P03": {}, // lock_not_available
	"40P01": {}, // deadlock_detected
	"40001": {}, // serialization_failure
}

// default max retry attempts
const defaultMaxLockRetries = 12

func execLockRetry(timeout time.Duration, maxRetries int) dbx.ExecHookFunc {
	return func(q *dbx.Query, op func() error) error {
		originalContext := q.Context()
		if originalContext == nil {
			cancelCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer func() {
				cancel()
				q.WithContext(context.TODO())
			}()
			q.WithContext(cancelCtx)
		}

		execErr := baseLockRetry(func(attempt int) error {
			return op()
		}, maxRetries)
		if execErr != nil && !errors.Is(execErr, sql.ErrNoRows) {
			execErr = fmt.Errorf("%w; failed query: %s", execErr, q.SQL())
		}

		return execErr
	}
}

func baseLockRetry(op func(attempt int) error, maxRetries int) error {
	attempt := 1

Retry:
	err := op(attempt)

	if err != nil && attempt <= maxRetries {
		if shouldRetryLockError(err) {
			// wait and retry
			time.Sleep(getDefaultRetryInterval(attempt))
			attempt++
			goto Retry
		}
	}

	return err
}

func shouldRetryLockError(err error) bool {
	if err == nil {
		return false
	}

	if code, ok := sqlStateFromError(err); ok {
		if _, ok := retryableSQLStates[code]; ok {
			return true
		}
	}

	errStr := strings.ToLower(err.Error())

	// SQLite / generic lock errors fallback.
	if strings.Contains(errStr, "database is locked") || strings.Contains(errStr, "table is locked") {
		return true
	}

	// PostgreSQL textual fallback in case SQLSTATE is not exposed by the driver.
	return strings.Contains(errStr, "deadlock detected") ||
		strings.Contains(errStr, "could not serialize access") ||
		strings.Contains(errStr, "lock not available") ||
		strings.Contains(errStr, "could not obtain lock on")
}

func sqlStateFromError(err error) (string, bool) {
	type sqlStateCarrier interface {
		SQLState() string
	}

	var stateErr sqlStateCarrier
	if errors.As(err, &stateErr) {
		code := strings.ToUpper(strings.TrimSpace(stateErr.SQLState()))
		return code, code != ""
	}

	return "", false
}

func getDefaultRetryInterval(attempt int) time.Duration {
	if attempt < 0 || attempt > len(defaultRetryIntervals)-1 {
		return time.Duration(defaultRetryIntervals[len(defaultRetryIntervals)-1]) * time.Millisecond
	}

	return time.Duration(defaultRetryIntervals[attempt]) * time.Millisecond
}
