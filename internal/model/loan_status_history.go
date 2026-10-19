package model

type LoanStatusHistory struct {
	ID            string `gorm:"type:bigint;primaryKey;autoIncrement" json:"id"`
	LoanNo        string `gorm:"column:loan_no;size:22;not null;index" json:"loan_no"`
	PreviousState Status
	NewState      Status
	Action        Action
	CreatedAt     int64 `gorm:"default:now()" json:"created_at"`
}

func (LoanStatusHistory) TableName() string { return "loan_status_history" }
