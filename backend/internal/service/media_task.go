package service

import (
	"context"
	"encoding/json"
	"time"
)

type MediaTask struct {
	ID                      int64
	PublicID                string
	UserID                  int64
	APIKeyID                int64
	GroupID                 int64
	ChannelID               *int64
	AccountID               *int64
	MediaType               MediaType
	Operation               MediaOperation
	RequestedModel          string
	UpstreamModel           string
	Adapter                 string
	NativeAsyncMode         NativeAsyncMode
	ClientAsync             bool
	SyncFallback            bool
	Status                  MediaTaskStatus
	Stage                   MediaTaskStage
	Progress                int
	RequestSpec             json.RawMessage
	ImageResponseFormat     string
	CandidateSnapshot       json.RawMessage
	RequestFingerprint      string
	IdempotencyKey          string
	UpstreamTaskID          string
	PollMetadata            json.RawMessage
	BillingSnapshot         json.RawMessage
	SettlementPlan          json.RawMessage
	SettlementRecovery      json.RawMessage
	BillingStatus           string
	PrechargedAmount        float64
	FinalAmount             float64
	RefundedAmount          float64
	AdditionalChargedAmount float64
	RetryCount              int
	ErrorCode               string
	ErrorMessage            string
	WorkerID                string
	ClaimToken              string
	LeaseUntil              *time.Time
	Version                 int64
	SubmittedAt             *time.Time
	StartedAt               *time.Time
	FinishedAt              *time.Time
	SyncFallbackAt          *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type MediaArtifact struct {
	ID              int64
	TaskID          int64
	Direction       string
	Position        int
	MediaType       MediaType
	ContentType     string
	SizeBytes       int64
	ChecksumSHA256  string
	Width           *int
	Height          *int
	DurationSeconds *float64
	Resolution      string
	FPS             *float64
	StorageStatus   string
	StorageProvider string
	// StorageRevision exists only between object Put and Artifact commit. It is
	// deliberately not persisted: the guarded commit prevents locator changes
	// once this artifact has been indexed.
	StorageRevision   string `json:"-"`
	ObjectKey         string
	PublicURL         string
	UpstreamReference string
	ExpiresAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type MediaTaskRepository interface {
	Create(ctx context.Context, task *MediaTask) (*MediaTask, error)
	GetByID(ctx context.Context, id int64) (*MediaTask, error)
	GetByPublicIDForUser(ctx context.Context, publicID string, userID, apiKeyID int64) (*MediaTask, error)
	GetByIdempotencyKey(ctx context.Context, userID, apiKeyID int64, key string) (*MediaTask, error)
	UpdateQueued(ctx context.Context, id, version int64, updates map[string]any) (bool, error)
	// TransitionQueued is the initialization-owner terminal path. It requires
	// the queued state and expected version in the same CAS update.
	TransitionQueued(ctx context.Context, id, expectedVersion int64, to MediaTaskStatus, updates map[string]any) (bool, error)
	Claim(ctx context.Context, id int64, workerID, claimToken string, leaseUntil time.Time, version int64) (bool, error)
	RenewLease(ctx context.Context, id int64, claimToken string, leaseUntil time.Time) (bool, error)
	UpdateClaimed(ctx context.Context, id int64, claimToken string, expectedVersion int64, expectedStage MediaTaskStage, updates map[string]any) (bool, error)
	// Transition is reserved for orchestrator/system state changes. It enforces the
	// domain state machine and status CAS, but intentionally does not assert Worker ownership.
	Transition(ctx context.Context, id int64, from, to MediaTaskStatus, updates map[string]any) (bool, error)
	// TransitionSyncTimeout is the fresh-snapshot timeout failure path. It requires
	// the expected version and sync_fallback=false without weakening Worker ownership transitions.
	TransitionSyncTimeout(ctx context.Context, id, expectedVersion int64, expectedStage MediaTaskStage, from MediaTaskStatus, updates map[string]any) (bool, error)
	// TransitionClaimed is the Worker completion/failure path. It additionally
	// requires the current Worker, expected version, and a live lease in one CAS update.
	TransitionClaimed(ctx context.Context, id int64, claimToken string, expectedVersion int64, expectedStage MediaTaskStage, from, to MediaTaskStatus, updates map[string]any) (bool, error)
	MarkSyncFallback(ctx context.Context, id int64, at time.Time) (bool, error)
	ListRecoverable(ctx context.Context, now time.Time, limit int) ([]MediaTask, error)
	// ListSettlementPending returns settlement work whose last state change is
	// old enough to retry. Implementations must order oldest work first so a
	// permanently failing task cannot starve later rows at the batch boundary.
	ListSettlementPending(ctx context.Context, retryReadyBefore time.Time, limit int) ([]MediaTask, error)
	UpdateBilling(ctx context.Context, id int64, fromStatus string, updates map[string]any) (bool, error)
}

type MediaArtifactRepository interface {
	Create(ctx context.Context, artifact *MediaArtifact) (*MediaArtifact, error)
	// DeleteExact removes exactly one row created by the current media workflow.
	// Implementations must match the immutable artifact identity in addition to
	// the row ID so compensation can never delete another task's artifact.
	DeleteExact(ctx context.Context, artifact *MediaArtifact) (bool, error)
	ListByTaskID(ctx context.Context, taskID int64) ([]MediaArtifact, error)
}
