package model

import "time"

// User representa um doutor do sistema. É quem faz login.
type User struct {
	ID        uint   `gorm:"primaryKey;autoincrement" json:"id"`
	Nome      string `gorm:"type:varchar(100);not null;index" json:"nome"`
	Hash_pass string `gorm:"type:varchar(255);not null" json:"-"`
	// SessaoEncerradaEm é quando o doutor finalizou o último plantão. Tokens
	// emitidos antes disso deixam de valer (o doutor é deslogado).
	SessaoEncerradaEm *time.Time `json:"-"`
}
