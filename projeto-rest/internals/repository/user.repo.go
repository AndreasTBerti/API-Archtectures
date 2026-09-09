package repository

import (
	"projeto-rest/internals/model"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type UserRepository interface {
	Findall() ([]model.User, error)
	FindById(id uint) (*model.User, error)
	FindByNome(nome string) (*model.User, error)
	Create(user model.User) (model.User, error)
	Update(user model.User) (model.User, error)
	Delete(user model.User) (model.User, error)
}

type gormUserRepository struct { db * gorm.DB }

func New(db * gorm.DB) UserRepository {return &gormUserRepository{db}}

func (r *gormUserRepository) Findall() ([]model.User, error) {
	var users []model.User
	err := r.db.Find(&users).Error
	return users, err 
}

func (r *gormUserRepository) FindById(id uint) (*model.User, error) {
	var user model.User
	if err := r.db.Find(&user, id).Error; err!= nil{
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

func (r *gormUserRepository) Create(user model.User) (model.User, error) {
	hashedPass, err := bcrypt.GenerateFromPassword([]byte(user.Hash_pass), bcrypt.DefaultCost)
	if err != nil {
		return user, err
	}
	user.Hash_pass = string(hashedPass)

	err = r.db.Create(&user).Error
	return user, err
}

func (r *gormUserRepository) Update(user model.User) (model.User, error) {
	err := r.db.Save(&user).Error
	return user, err
}

func (r *gormUserRepository) Delete(user model.User) (model.User, error) {
	err := r.db.Delete(&user).Error
	return user, err
}