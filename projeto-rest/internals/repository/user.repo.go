package repository

import (
	"time"

	"projeto-rest/internals/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserRepository interface {
	Findall() ([]model.User, error)
	FindById(id uint) (*model.User, error)
	FindByNome(nome string) (*model.User, error)
	// FindFirst devolve o primeiro doutor criado (menor ID).
	FindFirst() (*model.User, error)
	Create(user model.User) (model.User, error)
	Update(user model.User) (model.User, error)
	Delete(user model.User) (model.User, error)
	CheckPassword(user *model.User, senha string) bool
	// EncerrarSessao invalida os tokens do doutor emitidos antes de "em".
	EncerrarSessao(id uint, em time.Time) error
}

type gormUserRepository struct{ db *gorm.DB }

func New(db *gorm.DB) UserRepository { return &gormUserRepository{db} }

func (r *gormUserRepository) Findall() ([]model.User, error) {
	var users []model.User
	err := r.db.Find(&users).Error
	return users, err
}

func (r *gormUserRepository) FindById(id uint) (*model.User, error) {
	var user model.User
	if err := r.db.First(&user, id).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *gormUserRepository) FindByNome(nome string) (*model.User, error) {
	var user model.User
	if err := r.db.Where("nome = ?", nome).First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *gormUserRepository) FindFirst() (*model.User, error) {
	var user model.User
	if err := r.db.Order("id ASC").First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func hashPassword(senha string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(senha), bcrypt.DefaultCost)
	return string(hashed), err
}

// Create recebe a senha em texto puro em Hash_pass e a substitui pelo hash.
func (r *gormUserRepository) Create(user model.User) (model.User, error) {
	hashed, err := hashPassword(user.Hash_pass)
	if err != nil {
		return user, err
	}
	user.Hash_pass = hashed

	err = r.db.Create(&user).Error
	return user, err
}

// Update recebe a nova senha em texto puro em Hash_pass (ou vazio para manter a atual).
func (r *gormUserRepository) Update(user model.User) (model.User, error) {
	if user.Hash_pass != "" {
		hashed, err := hashPassword(user.Hash_pass)
		if err != nil {
			return user, err
		}
		user.Hash_pass = hashed
	} else {
		var atual model.User
		if err := r.db.First(&atual, user.ID).Error; err != nil {
			return user, err
		}
		user.Hash_pass = atual.Hash_pass
	}
	err := r.db.Save(&user).Error
	return user, err
}

func (r *gormUserRepository) Delete(user model.User) (model.User, error) {
	err := r.db.Delete(&user).Error
	return user, err
}

func (r *gormUserRepository) CheckPassword(user *model.User, senha string) bool {
	return bcrypt.CompareHashAndPassword([]byte(user.Hash_pass), []byte(senha)) == nil
}

func (r *gormUserRepository) EncerrarSessao(id uint, em time.Time) error {
	return r.db.Model(&model.User{}).Where("id = ?", id).Update("sessao_encerrada_em", em).Error
}
