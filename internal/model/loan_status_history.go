package model

type LoanStatusHistory struct {
	ID            int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	LoanNo        string `gorm:"column:loan_no;size:22;not null;index" json:"loan_no"`
	PreviousState Status `gorm:"column:previous_state;not null" json:"previous_state"`
	NewState      Status `gorm:"column:new_state;not null" json:"new_state"`
	Action        Action `gorm:"column:action;not null" json:"action"`
	CreatedAt     int64  `gorm:"autoCreateTime:milli" json:"created_at"`
}

func (LoanStatusHistory) TableName() string {
	return "loan_status_history"
}
