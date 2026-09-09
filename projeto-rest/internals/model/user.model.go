package model

type User struct {
	ID uint `gorm:"primaryKey;autoincrement"`
	Nome string `gorm:"type:varchar(100);not null"`
	Hash_pass string `gorm:"type:varchar(255);not null" json:"-"`
}