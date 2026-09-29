package handler

import (
	"net/http"

	"projeto-rest/internals/auth"
	"projeto-rest/internals/dto"
	"projeto-rest/internals/middleware"
	"projeto-rest/internals/model"
	"projeto-rest/internals/repository"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	repo repository.UserRepository
	jwt  *auth.Service
}

func NewAuthHandler(repo repository.UserRepository, jwt *auth.Service) *AuthHandler {
	return &AuthHandler{repo: repo, jwt: jwt}
}

// @Summary         Registrar doutor
// @Description     Cria um novo doutor (usuário) que poderá fazer login
// @Tags            Auth
// @Accept          json
// @Produce         json
// @Param           user body      dto.UserDTO true "Nome e senha"
// @Success         201  {object}  model.User
// @Failure         400  {object}  map[string]string
// @Failure         409  {object}  map[string]string
// @Router          /auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var body dto.UserDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if existing, _ := h.repo.FindByNome(body.Nome); existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "Usuário já existe"})
		return
	}

	created, err := h.repo.Create(model.User{Nome: body.Nome, Hash_pass: body.Senha})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

// @Summary         Obter token (login)
// @Description     Fluxo OAuth2 "password": valida nome/senha e devolve um JWT Bearer. Aceita JSON {nome, senha} ou form-urlencoded (grant_type=password&username=...&password=...).
// @Tags            Auth
// @Accept          json
// @Accept          x-www-form-urlencoded
// @Produce         json
// @Param           credentials body      dto.LoginDTO true "Credenciais"
// @Success         200  {object}  dto.TokenResponse
// @Failure         400  {object}  dto.OAuthError
// @Failure         401  {object}  dto.OAuthError
// @Router          /auth/token [post]
func (h *AuthHandler) Token(c *gin.Context) {
	var body dto.LoginDTO
	if err := c.ShouldBind(&body); err != nil {
		c.JSON(http.StatusBadRequest, dto.OAuthError{Error: "invalid_request", ErrorDescription: err.Error()})
		return
	}

	// No fluxo form-urlencoded do OAuth2, grant_type é obrigatório e deve ser "password".
	if gt := c.PostForm("grant_type"); gt != "" && gt != "password" {
		c.JSON(http.StatusBadRequest, dto.OAuthError{Error: "unsupported_grant_type"})
		return
	}

	user, err := h.repo.FindByNome(body.Nome)
	if err != nil || !h.repo.CheckPassword(user, body.Senha) {
		c.JSON(http.StatusUnauthorized, dto.OAuthError{Error: "invalid_grant", ErrorDescription: "nome ou senha inválidos"})
		return
	}

	token, err := h.jwt.Generate(user.ID, user.Nome)
	if err != nil {
		c.JSON(http.StatusInternalServerError, dto.OAuthError{Error: "server_error", ErrorDescription: err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.TokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(h.jwt.TTL().Seconds()),
	})
}

// @Summary         Usuário autenticado
// @Description     Retorna o doutor dono do token enviado
// @Tags            Auth
// @Produce         json
// @Security        BearerAuth
// @Success         200  {object}  model.User
// @Failure         401  {object}  map[string]string
// @Router          /auth/me [get]
func (h *AuthHandler) Me(c *gin.Context) {
	user, err := h.repo.FindById(middleware.CurrentUserID(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "usuário do token não existe mais"})
		return
	}
	c.JSON(http.StatusOK, user)
}
