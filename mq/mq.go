package mq

import "context"

const (
	// SyncChannelKey is key name for sync message channel
	SyncChannelKey = "checker:channel"
	// SyncPendingChannelKey is key name for sync started but does not complete
	// message channel
	SyncPendingChannelKey = "checker:channel:pending"
	// SyncErrorChannelKey is key name for sync error message channel
	SyncErrorChannelKey = "checker:channel:error"
	// SyncRetriesChannelKey is key name for store sync error times
	SyncRetriesChannelKey = "checker:channel:retries"
)

// MessageQueue interface
type MessageQueue interface {
	Init() error
	Reset()
	Push(message string, removePrefix string) error
	Subscribe(ctx context.Context) (string, error)
	GetN(ctx context.Context, n int, running []string) ([]string, error)
	GetNWithin(ctx context.Context, n int, running []string, within []string) ([]string, error)
	Finish(message string) error
	Error(message string) error
	MoveAllPendingToError() (int, error)
	MoveErrorToPending() (string, error)
	GetErrorTimes(message string) (int64, error)
	Retry(message string) error

	Exists(message string) (bool, error)
}
