package handler

import (
	"errors"
	"net/http"
	"strconv"

	"projeto-rest/internals/dto"
	"projeto-rest/internals/model"
	"projeto-rest/internals/pacientes"
	"projeto-rest/internals/repository"
	"projeto-rest/internals/soap"

	"github.com/gin-gonic/gin"
)

// Notifier avisa o quadro em tempo real (WebSocket) sobre mudanças.
type Notifier interface {
	NotifyPaciente(p model.Paciente)
	NotifyPacienteRemovido(id uint, passagens []uint)
	NotifyPassagem(p model.Passagem)
	ReleaseLock(passagemID uint)
	DesconectarDoutor(doutorID uint)
}

// PacienteHandler expõe os pacientes pela API REST. Os dados moram no
// serviço SOAP: toda operação aqui é repassada a ele. O REST só cuida de
// apagar os próprios documentos quando um paciente é removido.
type PacienteHandler struct {
	dir       pacientes.Diretorio
	passagens repository.PassagemRepository
	notify    Notifier
}

func NewPacienteHandler(dir pacientes.Diretorio, passagens repository.PassagemRepository, notify Notifier) *PacienteHandler {
	return &PacienteHandler{dir: dir, passagens: passagens, notify: notify}
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID inválido"})
		return 0, false
	}
	return uint(id), true
}

// erroSoap traduz o erro do serviço SOAP em resposta HTTP.
func erroSoap(c *gin.Context, err error) {
	var validacao *soap.ErroValidacao
	switch {
	case errors.As(err, &validacao):
		c.JSON(http.StatusBadRequest, gin.H{"error": validacao.Mensagem})
	case errors.Is(err, pacientes.ErrNaoEncontrado):
		c.JSON(http.StatusNotFound, gin.H{"error": "Paciente não encontrado"})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"error": "Serviço SOAP de pacientes indisponível"})
	}
}

// @Summary         Listar pacientes
// @Description     Lista os pacientes do serviço SOAP (list_pacientes), em ordem alfabética
// @Tags            Pacientes
// @Produce         json
// @Security        BearerAuth
// @Success         200  {array}   model.Paciente
// @Failure         401  {object}  map[string]string
// @Failure         502  {object}  map[string]string
// @Router          /pacientes [get]
func (h *PacienteHandler) Findall(c *gin.Context) {
	lista, err := h.dir.Listar()
	if err != nil {
		erroSoap(c, err)
		return
	}
	c.JSON(http.StatusOK, lista)
}

// @Summary         Buscar paciente
// @Description     Busca um paciente no serviço SOAP (get_paciente)
// @Tags            Pacientes
// @Produce         json
// @Security        BearerAuth
// @Param           id   path      int  true  "id_paciente no SOAP"
// @Success         200  {object}  model.Paciente
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Failure         502  {object}  map[string]string
// @Router          /pacientes/{id} [get]
func (h *PacienteHandler) FindById(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	p, err := h.dir.Buscar(id)
	if err != nil {
		erroSoap(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

// @Summary         Cadastrar paciente
// @Description     Cria o paciente no serviço SOAP (criar_paciente). Cadastrar não coloca o paciente no quadro: para isso, registre a chegada.
// @Tags            Pacientes
// @Accept          json
// @Produce         json
// @Security        BearerAuth
// @Param           paciente body  dto.PacienteDTO  true  "Dados do paciente"
// @Success         201  {object}  model.Paciente
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         502  {object}  map[string]string
// @Router          /pacientes [post]
func (h *PacienteHandler) Create(c *gin.Context) {
	var body dto.PacienteDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Informe o nome do paciente"})
		return
	}
	p, err := h.dir.Criar(body.Nome, body.Status)
	if err != nil {
		erroSoap(c, err)
		return
	}
	h.notify.NotifyPaciente(*p)
	c.JSON(http.StatusCreated, p)
}

// @Summary         Atualizar paciente
// @Description     Atualiza nome e status no serviço SOAP (atualizar_paciente)
// @Tags            Pacientes
// @Accept          json
// @Produce         json
// @Security        BearerAuth
// @Param           id       path  int              true  "id_paciente no SOAP"
// @Param           paciente body  dto.PacienteDTO  true  "Dados do paciente"
// @Success         200  {object}  model.Paciente
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Failure         502  {object}  map[string]string
// @Router          /pacientes/{id} [put]
func (h *PacienteHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var body dto.PacienteDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Informe o nome do paciente"})
		return
	}
	p, err := h.dir.Atualizar(id, body.Nome, body.Status)
	if err != nil {
		erroSoap(c, err)
		return
	}
	h.notify.NotifyPaciente(*p)
	c.JSON(http.StatusOK, p)
}

// @Summary         Remover paciente
// @Description     Remove o paciente no serviço SOAP (remover_paciente) e apaga as passagens e documentos dele no REST. Não é permitido enquanto ele estiver no hospital.
// @Tags            Pacientes
// @Produce         json
// @Security        BearerAuth
// @Param           id   path      int  true  "id_paciente no SOAP"
// @Success         200  {object}  model.Paciente
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Failure         409  {object}  map[string]string
// @Failure         502  {object}  map[string]string
// @Router          /pacientes/{id} [delete]
func (h *PacienteHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if _, err := h.passagens.FindAberta(id); err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "O paciente está no hospital. Registre a alta antes de removê-lo."})
		return
	}
	p, err := h.dir.Remover(id)
	if err != nil {
		erroSoap(c, err)
		return
	}
	ids, err := h.passagens.DeleteByPaciente(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.notify.NotifyPacienteRemovido(id, ids)
	c.JSON(http.StatusOK, p)
}
