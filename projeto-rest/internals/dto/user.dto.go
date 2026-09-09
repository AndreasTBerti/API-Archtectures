package dto

type UserDTO struct {
	ID uint `json:"id"`
	Nome string `json:"nome" binding:"required"`
	Hash_pass string `json: "hash_pass" binding:"required"`
}

