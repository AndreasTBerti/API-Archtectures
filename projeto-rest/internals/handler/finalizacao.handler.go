package handler

import (
	"net/http"
	"strings"
	"time"

	"projeto-rest/internals/dto"
	"projeto-rest/internals/middleware"
	"projeto-rest/internals/model"
	"projeto-rest/internals/pacientes"
	"projeto-rest/internals/repository"

	"github.com/gin-gonic/gin"
)

const maxComentario = 2000

// FinalizacaoHandler fecha o plantão do doutor: mostra tudo o que ele
// registrou desde a última finalização, grava os comentários dele e encerra
// a sessão (o token deixa de valer).
type FinalizacaoHandler struct {
	movs      repository.FinalizacaoRepository
	users     repository.UserRepository
	passagens repository.PassagemRepository
	pacientes pacientes.Diretorio
	notify    Notifier
}

func NewFinalizacaoHandler(movs repository.FinalizacaoRepository, users repository.UserRepository, passagens repository.PassagemRepository, dir pacientes.Diretorio, notify Notifier) *FinalizacaoHandler {
	return &FinalizacaoHandler{movs: movs, users: users, passagens: passagens, pacientes: dir, notify: notify}
}

// previa monta o resumo das movimentações pendentes do doutor. O plantão
// começa na primeira movimentação pendente, ou no login se não houver nenhuma.
func (h *FinalizacaoHandler) previa(c *gin.Context) (dto.PreviaFinalizacao, error) {
	doutor := middleware.CurrentUserID(c)
	ms, err := h.movs.Pendentes(doutor)
	if err != nil {
		return dto.PreviaFinalizacao{}, err
	}
	p := dto.PreviaFinalizacao{
		Inicio:        middleware.EmitidoEm(c),
		Fim:           time.Now(),
		Totais:        map[string]int{},
		Movimentacoes: ms,
	}
	pacientesVistos := map[uint]bool{}
	for _, m := range ms {
		p.Totais[m.Tipo]++
		pacientesVistos[m.PacienteID] = true
		if m.CreatedAt.Before(p.Inicio) {
			p.Inicio = m.CreatedAt
		}
	}
	p.Pacientes = len(pacientesVistos)
	return p, nil
}

// @Summary         Resumo para finalizar o plantão
// @Description     Tudo o que o doutor autenticado registrou (chegadas, triagens, consultas, correções) desde a última finalização. É o que ele revisa e comenta antes de finalizar.
// @Tags            Finalização
// @Produce         json
// @Security        BearerAuth
// @Success         200  {object}  dto.PreviaFinalizacao
// @Failure         401  {object}  map[string]string
// @Router          /finalizacao [get]
func (h *FinalizacaoHandler) Previa(c *gin.Context) {
	p, err := h.previa(c)
	if err != nil {
		erro500(c, err)
		return
	}
	c.JSON(http.StatusOK, p)
}

// @Summary         Finalizar o plantão
// @Description     Grava a finalização com o comentário geral e os comentários opcionais de cada movimentação, e encerra a sessão do doutor: o token atual deixa de valer e o quadro é desconectado.
// @Tags            Finalização
// @Accept          json
// @Produce         json
// @Security        BearerAuth
// @Param           finalizacao  body  dto.FinalizarDTO  true  "Comentários do doutor"
// @Success         201  {object}  model.Finalizacao
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Router          /finalizacao [post]
func (h *FinalizacaoHandler) Finalizar(c *gin.Context) {
	var body dto.FinalizarDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	comentarios := map[uint]string{}
	for _, cm := range body.Comentarios {
		texto := strings.TrimSpace(cm.Comentario)
		if len([]rune(texto)) > maxComentario {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Comentário muito longo"})
			return
		}
		if texto != "" {
			comentarios[cm.MovimentacaoID] = texto
		}
	}
	geral := strings.TrimSpace(body.Comentario)
	if len([]rune(geral)) > maxComentario {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Comentário muito longo"})
		return
	}

	previa, err := h.previa(c)
	if err != nil {
		erro500(c, err)
		return
	}
	doutor := middleware.CurrentUserID(c)
	f := model.Finalizacao{DoutorID: doutor, Inicio: previa.Inicio, Fim: previa.Fim, Comentario: geral}
	if err := h.movs.Finalizar(&f, comentarios); err != nil {
		erro500(c, err)
		return
	}
	if err := h.users.EncerrarSessao(doutor, f.Fim); err != nil {
		erro500(c, err)
		return
	}

	// Os comentários passam a aparecer nas passagens: avisa o quadro.
	avisadas := map[uint]bool{}
	for _, m := range previa.Movimentacoes {
		if comentarios[m.ID] == "" || avisadas[m.PassagemID] {
			continue
		}
		avisadas[m.PassagemID] = true
		if pg, err := h.passagens.FindById(m.PassagemID); err == nil {
			pacientes.AnexarUma(h.pacientes, pg)
			h.notify.NotifyPassagem(*pg)
		}
	}
	h.notify.DesconectarDoutor(doutor)

	salva, err := h.movs.FindById(f.ID)
	if err != nil {
		erro500(c, err)
		return
	}
	c.JSON(http.StatusCreated, salva)
}

// @Summary         Minhas finalizações
// @Description     Plantões já finalizados pelo doutor autenticado, do mais recente para o mais antigo
// @Tags            Finalização
// @Produce         json
// @Security        BearerAuth
// @Success         200  {array}   model.Finalizacao
// @Failure         401  {object}  map[string]string
// @Router          /finalizacoes [get]
func (h *FinalizacaoHandler) Listar(c *gin.Context) {
	fs, err := h.movs.ListarDoDoutor(middleware.CurrentUserID(c))
	if err != nil {
		erro500(c, err)
		return
	}
	c.JSON(http.StatusOK, fs)
}
