//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"LoanService/internal/model"
	"LoanService/internal/notification"
	"LoanService/internal/repository"
	"github.com/stretchr/testify/require"
)

type fakeSender struct {
	fail  bool
	calls int
}

func (s *fakeSender) Send(context.Context, string, string, string) error {
	s.calls++
	if s.fail {
		return errors.New("mail unavailable")
	}
	return nil
}
func TestOutboxWorkerRetryAndDelivery(t *testing.T) {
	s, db := setup(t)
	l := create(t, s, 100)
	approve(t, s, l.LoanNo)
	_, err := s.Invest(context.Background(), l.LoanNo, "investor", inv("fund", 100))
	require.NoError(t, err)
	sender := &fakeSender{fail: true}
	worker := notification.Worker{DB: db, Sender: sender}
	c := context.Background()
	t.Log("[Run] Mock mail failure; want mail unavailable, lease cleared, retry delayed")
	require.EqualError(t, worker.Once(c), "mail unavailable")
	var task model.NotificationOutbox
	require.NoError(t, db.Take(&task).Error)
	t.Logf("[Task] ID=%d, attempts=%d, available_at=%d, delivered_at=%v, lease=%v", task.ID, task.Attempts, task.AvailableAt, task.DeliveredAt, task.LeaseToken)
	require.Nil(t, task.DeliveredAt)
	require.Nil(t, task.LeaseToken)
	require.Equal(t, 1, task.Attempts)
	require.Greater(t, task.AvailableAt, time.Now().UnixMilli())
	t.Log("[Run] Run worker before retry time; want no mail sent")
	require.NoError(t, worker.Once(c))
	t.Logf("[Check] Send calls=%d, want=1", sender.calls)
	require.Equal(t, 1, sender.calls)
	require.NoError(t, db.Model(&task).Update("available_at", 0).Error)
	t.Log("[Run] Make task ready and mock mail success; run twice, want only 1 new send")
	sender.fail = false
	require.NoError(t, worker.Once(c))
	require.NoError(t, worker.Once(c))
	t.Logf("[Check] Total send calls=%d, want=2 (1 failure + 1 success)", sender.calls)
	require.Equal(t, 2, sender.calls)
	var delivered model.NotificationOutbox
	require.NoError(t, db.Take(&delivered).Error)
	t.Logf("[Done] attempts=%d, delivered_at=%v, lease_token=%v; want 2 attempts, delivered, no lease", delivered.Attempts, delivered.DeliveredAt, delivered.LeaseToken)
	require.NotNil(t, delivered.DeliveredAt)
	require.Nil(t, delivered.LeaseToken)
	require.Equal(t, 2, delivered.Attempts)
}
func TestOutboxConcurrentClaimAndExpiredLease(t *testing.T) {
	s, db := setup(t)
	l := create(t, s, 100)
	approve(t, s, l.LoanNo)
	_, err := s.Invest(context.Background(), l.LoanNo, "investor", inv("fund", 100))
	require.NoError(t, err)
	repo := repository.NewNotificationOutboxRepository(repository.NewRepository(db))
	c := context.Background()
	claimed := make([]*model.NotificationOutbox, 16)
	t.Log("[Run] 16 workers claim one task together; want only 1 claim")
	errs := concurrently(16, func(i int) error { var err error; claimed[i], err = repo.Claim(c, fmt.Sprint(i)); return err })
	n := 0
	var taskID int64
	for i, err := range errs {
		t.Logf("[Claim] Worker[%d] task=%+v, error=%v", i, claimed[i], err)
		require.NoError(t, err)
		if claimed[i] != nil {
			n++
			taskID = claimed[i].ID
		}
	}
	t.Logf("[Check] Claimed count=%d, want=1", n)
	require.Equal(t, 1, n)
	var task model.NotificationOutbox
	require.NoError(t, db.Take(&task).Error)
	t.Logf("[Task] ID=%d, attempts=%d, available_at=%d, delivered_at=%v, lease=%v", task.ID, task.Attempts, task.AvailableAt, task.DeliveredAt, task.LeaseToken)
	require.Equal(t, 1, task.Attempts)
	require.NotNil(t, task.LeaseToken)
	oldLease := *task.LeaseToken
	require.NoError(t, db.Model(&task).Update("available_at", 0).Error)
	t.Log("[Run] Expire lease and claim with new-lease; want the same task")
	reclaimed, err := repo.Claim(c, "new-lease")
	t.Logf("[Claim again] task=%+v, error=%v", reclaimed, err)
	require.NoError(t, err)
	require.NotNil(t, reclaimed)
	require.Equal(t, taskID, reclaimed.ID)
	t.Log("[Run] Complete and retry with old lease; want no change to new lease")
	require.NoError(t, repo.Complete(c, taskID, oldLease))
	require.NoError(t, repo.Retry(c, taskID, oldLease))
	var current model.NotificationOutbox
	require.NoError(t, db.Take(&current).Error)
	require.Nil(t, current.DeliveredAt)
	require.Equal(t, "new-lease", *current.LeaseToken)
	require.Equal(t, 2, current.Attempts)
	t.Log("[Run] Complete with new lease; want delivered_at set and no more claims")
	require.NoError(t, repo.Complete(c, taskID, "new-lease"))
	var done model.NotificationOutbox
	require.NoError(t, db.Take(&done).Error)
	require.NotNil(t, done.DeliveredAt)
	empty, err := repo.Claim(c, "later")
	require.NoError(t, err)
	t.Logf("[Final check] Claim again: task=%+v, error=%v; want no task", empty, err)
	require.Nil(t, empty)
}
