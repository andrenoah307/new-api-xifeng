package model

// EmailNotification is an outbox for asynchronous notifications. Message bodies
// are private, removed after delivery, and retained at most 24 hours on failure.
type EmailNotification struct {
	ID        string `gorm:"primaryKey;type:varchar(36)"`
	Status    string `gorm:"type:varchar(16);index:idx_email_notification_due,priority:1"`
	NextAt    int64  `gorm:"index:idx_email_notification_due,priority:2"`
	Attempts  int
	AttemptID string `gorm:"type:varchar(36)"`
	CreatedAt int64  `gorm:"index"`
	UpdatedAt int64  `gorm:"index"`
	Payload   string `json:"-" gorm:"size:4194304"`
}
