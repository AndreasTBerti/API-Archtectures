package dto

// UserDTO é o corpo usado para criar/atualizar um usuário (doutor).
type UserDTO struct {
	Nome  string `json:"nome" binding:"required"`
	Senha string `json:"senha" binding:"required"`
}
