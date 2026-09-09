package handler

import (
	"net/http"
	"strconv"

	"projeto-rest/internals/dto"
	"projeto-rest/internals/model"
	"projeto-rest/internals/repository"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	repo repository.UserRepository
}

func NewUserHandler(repo repository.UserRepository) *UserHandler {
	return &UserHandler{repo: repo}
}

// @Summary         Listar todos os usuários
// @Description     Retorna uma lista de todos os usuários cadastrados
// @Tags            Users
// @Produce         json
// @Success         200  {array}   model.User
// @Router          /users [get]
func (h *UserHandler) Findall(c *gin.Context) {
	users, err := h.repo.Findall()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

// @Summary         Buscar usuário por ID
// @Description     Busca um único usuário pelo ID fornecido
// @Tags            Users
// @Produce         json
// @Param           id   path      int  true  "ID do Usuário"
// @Success         200  {object}  model.User
// @Failure         400  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Router          /users/{id} [get]
func (h *UserHandler) FindById(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	user, err := h.repo.FindById(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}
	c.JSON(http.StatusOK, user)
}

// @Summary         Criar um usuário
// @Description     Cria um novo usuário na base de dados
// @Tags            Users
// @Accept          json
// @Produce         json
// @Param           user body      dto.UserDTO true "Dados do usuário"
// @Success         201  {object}  model.User
// @Failure         400  {object}  map[string]string
// @Router          /users [post]
func (h *UserHandler) Create(c *gin.Context) {
	var userDTO dto.UserDTO
	if err := c.ShouldBindJSON(&userDTO); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user := model.User{
		Nome:      userDTO.Nome,
		Hash_pass: userDTO.Hash_pass,
	}

	// Verifica se o usuário já existe no banco de forma otimizada
	existingUser, _ := h.repo.FindByNome(user.Nome)
	if existingUser != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Usuário já existe"})
		return
	}
	
	createdUser, err := h.repo.Create(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, createdUser)
}

// @Summary         Atualizar um usuário
// @Description     Atualiza os dados de um usuário existente
// @Tags            Users
// @Accept          json
// @Produce         json
// @Param           id   path      int  true  "ID do Usuário"
// @Param           user body      dto.UserDTO true "Dados do usuário para atualizar"
// @Success         200  {object}  model.User
// @Failure         400  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Router          /users/{id} [put]
func (h *UserHandler) Update(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	// Verificar se existe
	existingUser, err := h.repo.FindById(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	var userDTO dto.UserDTO
	if err := c.ShouldBindJSON(&userDTO); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existingUser.Nome = userDTO.Nome
	existingUser.Hash_pass = userDTO.Hash_pass

	updatedUser, err := h.repo.Update(*existingUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updatedUser)
}

// @Summary         Deletar um usuário
// @Description     Remove um usuário existente pelo ID
// @Tags            Users
// @Produce         json
// @Param           id   path      int  true  "ID do Usuário"
// @Success         200  {object}  model.User
// @Failure         400  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Router          /users/{id} [delete]
func (h *UserHandler) Delete(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return
	}

	// Verificar se existe
	existingUser, err := h.repo.FindById(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Usuário não encontrado"})
		return
	}

	deletedUser, err := h.repo.Delete(*existingUser)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, deletedUser)
}
