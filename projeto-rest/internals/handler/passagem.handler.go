package handler

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"projeto-rest/internals/dto"
	"projeto-rest/internals/middleware"
	"projeto-rest/internals/model"
	"projeto-rest/internals/pacientes"
	"projeto-rest/internals/repository"
	"projeto-rest/internals/resumo"

	"github.com/gin-gonic/gin"
)

// PassagemHandler cuida das vindas do paciente ao hospital: chegada,
// documentos de cada etapa, histórico e resumo. Os documentos ficam no banco
// do REST; os dados do paciente vêm do serviço SOAP.
type PassagemHandler struct {
	passagens repository.PassagemRepository
	pacientes pacientes.Diretorio
	movs      repository.FinalizacaoRepository
	notify    Notifier
	gerador   resumo.Gerador
}

func NewPassagemHandler(passagens repository.PassagemRepository, dir pacientes.Diretorio, movs repository.FinalizacaoRepository, notify Notifier, gerador resumo.Gerador) *PassagemHandler {
	return &PassagemHandler{passagens: passagens, pacientes: dir, movs: movs, notify: notify, gerador: gerador}
}

func (h *PassagemHandler) carregarPaciente(c *gin.Context) (*model.Paciente, bool) {
	id, ok := parseID(c)
	if !ok {
		return nil, false
	}
	p, err := h.pacientes.Buscar(id)
	if err != nil {
		erroSoap(c, err)
		return nil, false
	}
	return p, true
}

// registrar grava a movimentação do doutor autenticado, que depois aparece
// no resumo de finalização do plantão.
func (h *PassagemHandler) registrar(c *gin.Context, pg *model.Passagem, tipo, descricao string) {
	nome := ""
	if pg.Paciente != nil {
		nome = pg.Paciente.Nome
	}
	if err := h.movs.Registrar(model.Movimentacao{
		DoutorID:     middleware.CurrentUserID(c),
		PassagemID:   pg.ID,
		PacienteID:   pg.PacienteID,
		PacienteNome: nome,
		Tipo:         tipo,
		Descricao:    descricao,
	}); err != nil {
		log.Println("movimentação não registrada:", err)
	}
}

var formaChegadaLabel = map[string]string{
	"andando": "andando", "cadeira_rodas": "em cadeira de rodas", "maca": "de maca", "ambulancia": "de ambulância",
}

var prioridadeLabel = map[string]string{
	"baixa": "baixa", "media": "média", "alta": "alta", "emergencia": "emergência",
}

func semPonto(s string) string { return strings.TrimRight(strings.TrimSpace(s), ".") }

func descreverAdmissao(prefixo string, b dto.AdmissaoDTO) string {
	return fmt.Sprintf("%s Queixa: %s. Chegou %s.", prefixo, semPonto(b.QueixaPrincipal), formaChegadaLabel[b.FormaChegada])
}

func descreverTriagem(prefixo string, b dto.TriagemDTO) string {
	return fmt.Sprintf("%s prioridade %s, PA %s, %.1f °C. Sintomas: %s.",
		prefixo, prioridadeLabel[b.Prioridade], b.PressaoArterial, b.Temperatura, semPonto(b.Sintomas))
}

func descreverConsulta(b dto.ConsultaDTO) string {
	d := fmt.Sprintf("Consulta e alta. Diagnóstico: %s.", semPonto(b.Diagnostico))
	if strings.TrimSpace(b.Prescricao) != "" {
		d += fmt.Sprintf(" Prescrição: %s.", semPonto(b.Prescricao))
	}
	if strings.TrimSpace(b.Encaminhamento) != "" {
		d += fmt.Sprintf(" Encaminhamento: %s.", semPonto(b.Encaminhamento))
	}
	return d
}

// carregarAberta busca a passagem do path e exige que ela esteja aberta.
func (h *PassagemHandler) carregarAberta(c *gin.Context) (*model.Passagem, bool) {
	id, ok := parseID(c)
	if !ok {
		return nil, false
	}
	pg, err := h.passagens.FindById(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Passagem não encontrada"})
		return nil, false
	}
	if !pg.Aberta() {
		c.JSON(http.StatusConflict, gin.H{"error": "Esta passagem já foi encerrada"})
		return nil, false
	}
	pacientes.AnexarUma(h.pacientes, pg)
	return pg, true
}

// responder recarrega a passagem, libera o card, avisa o quadro e responde.
func (h *PassagemHandler) responder(c *gin.Context, id uint, status int) {
	h.notify.ReleaseLock(id)
	pg, err := h.passagens.FindById(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pacientes.AnexarUma(h.pacientes, pg)
	h.notify.NotifyPassagem(*pg)
	c.JSON(status, pg)
}

func erro500(c *gin.Context, err error) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func dataBR(data string) string {
	if t, err := time.Parse(model.FormatoData, data); err == nil {
		return t.Format("02/01/2006")
	}
	return data
}

// @Summary         Quadro
// @Description     Passagens abertas (de qualquer dia) mais as de hoje que já tiveram alta. É o mesmo conteúdo que o WebSocket envia no "init".
// @Tags            Passagens
// @Produce         json
// @Security        BearerAuth
// @Success         200  {array}   model.Passagem
// @Failure         401  {object}  map[string]string
// @Router          /quadro [get]
func (h *PassagemHandler) Quadro(c *gin.Context) {
	ps, err := h.passagens.FindQuadro(model.Hoje())
	if err != nil {
		erro500(c, err)
		return
	}
	h.pacientes.Anexar(ps)
	c.JSON(http.StatusOK, ps)
}

// @Summary         Passagens do paciente
// @Description     Lista todas as vindas do paciente ao hospital, com os documentos de cada uma, da mais recente para a mais antiga
// @Tags            Passagens
// @Produce         json
// @Security        BearerAuth
// @Param           id   path  int  true  "ID do paciente (local)"
// @Success         200  {array}   model.Passagem
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Router          /pacientes/{id}/passagens [get]
func (h *PassagemHandler) ListarDoPaciente(c *gin.Context) {
	p, ok := h.carregarPaciente(c)
	if !ok {
		return
	}
	ps, err := h.passagens.FindByPaciente(p.ID)
	if err != nil {
		erro500(c, err)
		return
	}
	for i := range ps {
		ps[i].Paciente = p
	}
	c.JSON(http.StatusOK, ps)
}

// @Summary         Registrar chegada
// @Description     Abre a passagem de hoje com a admissão e coloca o card em "Aguardando triagem". Se o paciente já teve alta hoje, a passagem do dia é reaberta. Responde 409 se ele já está no hospital.
// @Tags            Passagens
// @Accept          json
// @Produce         json
// @Security        BearerAuth
// @Param           id        path  int              true  "ID do paciente (local)"
// @Param           admissao  body  dto.AdmissaoDTO  true  "Admissão da chegada"
// @Success         201  {object}  model.Passagem
// @Success         200  {object}  model.Passagem  "Passagem do dia reaberta"
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Failure         409  {object}  map[string]string
// @Router          /pacientes/{id}/passagens [post]
func (h *PassagemHandler) RegistrarChegada(c *gin.Context) {
	p, ok := h.carregarPaciente(c)
	if !ok {
		return
	}
	var body dto.AdmissaoDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	hoje := model.Hoje()
	if aberta, err := h.passagens.FindAberta(p.ID); err == nil {
		msg := "O paciente já está no hospital"
		if aberta.Data != hoje {
			msg = fmt.Sprintf("O paciente ainda tem a passagem de %s aberta. Registre a consulta dela antes.", dataBR(aberta.Data))
		}
		c.JSON(http.StatusConflict, gin.H{"error": msg})
		return
	}

	n, _ := h.passagens.CountByEtapa(model.EtapaAguardandoTriagem)
	x, y := model.PosicaoPadrao(model.EtapaAguardandoTriagem, n)
	agora := time.Now()

	status, tipo := http.StatusCreated, model.MovChegada
	pg, err := h.passagens.FindByPacienteData(p.ID, hoje)
	if err != nil {
		pg = &model.Passagem{
			PacienteID: p.ID,
			Data:       hoje,
			Etapa:      model.EtapaAguardandoTriagem,
			PosX:       x,
			PosY:       y,
			ChegadaEm:  agora,
		}
		if err := h.passagens.Create(pg); err != nil {
			erro500(c, err)
			return
		}
	} else {
		// Voltou no mesmo dia depois da alta: reabre a passagem do dia.
		status, tipo = http.StatusOK, model.MovNovaChegada
		if err := h.passagens.Update(pg.ID, map[string]any{
			"etapa": model.EtapaAguardandoTriagem, "alta_em": nil,
			"pos_x": x, "pos_y": y, "chegada_em": agora,
		}); err != nil {
			erro500(c, err)
			return
		}
	}

	if err := h.passagens.UpsertAdmissao(model.Admissao{
		PassagemID:      pg.ID,
		AutorID:         middleware.CurrentUserID(c),
		QueixaPrincipal: body.QueixaPrincipal,
		FormaChegada:    body.FormaChegada,
		Acompanhante:    body.Acompanhante,
		Convenio:        body.Convenio,
		Observacoes:     body.Observacoes,
	}); err != nil {
		erro500(c, err)
		return
	}
	pg.Paciente = p
	if tipo == model.MovChegada {
		h.registrar(c, pg, tipo, descreverAdmissao("Chegada registrada.", body))
	} else {
		h.registrar(c, pg, tipo, descreverAdmissao("Voltou no mesmo dia depois da alta.", body))
	}
	h.responder(c, pg.ID, status)
}

// @Summary         Resumo do paciente
// @Description     Resumo para os doutores baseado só nas passagens encerradas de dias anteriores. A passagem atual nunca entra. Hoje o texto é gerado por regras; o contrato já está pronto para um gerador com IA.
// @Tags            Passagens
// @Produce         json
// @Security        BearerAuth
// @Param           id   path  int  true  "ID do paciente (local)"
// @Success         200  {object}  dto.ResumoResponse
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Router          /pacientes/{id}/resumo [get]
func (h *PassagemHandler) Resumo(c *gin.Context) {
	p, ok := h.carregarPaciente(c)
	if !ok {
		return
	}
	anteriores, err := h.passagens.FindAnteriores(p.ID, model.Hoje())
	if err != nil {
		erro500(c, err)
		return
	}
	c.JSON(http.StatusOK, resumo.Montar(h.gerador, *p, anteriores))
}

// @Summary         Buscar passagem
// @Description     Uma passagem com o paciente e os documentos
// @Tags            Passagens
// @Produce         json
// @Security        BearerAuth
// @Param           id   path  int  true  "ID da passagem"
// @Success         200  {object}  model.Passagem
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Router          /passagens/{id} [get]
func (h *PassagemHandler) FindById(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	pg, err := h.passagens.FindById(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Passagem não encontrada"})
		return
	}
	c.JSON(http.StatusOK, pg)
}

// @Summary         Corrigir admissão
// @Description     Atualiza a admissão de uma passagem aberta. Não muda a etapa.
// @Tags            Passagens
// @Accept          json
// @Produce         json
// @Security        BearerAuth
// @Param           id        path  int              true  "ID da passagem"
// @Param           admissao  body  dto.AdmissaoDTO  true  "Admissão"
// @Success         200  {object}  model.Passagem
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Failure         409  {object}  map[string]string
// @Router          /passagens/{id}/admissao [post]
func (h *PassagemHandler) Admissao(c *gin.Context) {
	pg, ok := h.carregarAberta(c)
	if !ok {
		return
	}
	var body dto.AdmissaoDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.passagens.UpsertAdmissao(model.Admissao{
		PassagemID:      pg.ID,
		AutorID:         middleware.CurrentUserID(c),
		QueixaPrincipal: body.QueixaPrincipal,
		FormaChegada:    body.FormaChegada,
		Acompanhante:    body.Acompanhante,
		Convenio:        body.Convenio,
		Observacoes:     body.Observacoes,
	}); err != nil {
		erro500(c, err)
		return
	}
	h.registrar(c, pg, model.MovCorrecaoAdmissao, descreverAdmissao("Admissão corrigida.", body))
	h.responder(c, pg.ID, http.StatusOK)
}

// @Summary         Registrar triagem
// @Description     Salva a triagem da passagem. Se ela estava em "aguardando_triagem", passa para "aguardando_consulta". pos_x/pos_y opcionais posicionam o card.
// @Tags            Passagens
// @Accept          json
// @Produce         json
// @Security        BearerAuth
// @Param           id       path  int             true  "ID da passagem"
// @Param           triagem  body  dto.TriagemDTO  true  "Triagem"
// @Success         200  {object}  model.Passagem
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Failure         409  {object}  map[string]string
// @Router          /passagens/{id}/triagem [post]
func (h *PassagemHandler) Triagem(c *gin.Context) {
	pg, ok := h.carregarAberta(c)
	if !ok {
		return
	}
	var body dto.TriagemDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.passagens.UpsertTriagem(model.Triagem{
		PassagemID:         pg.ID,
		AutorID:            middleware.CurrentUserID(c),
		PressaoArterial:    body.PressaoArterial,
		Temperatura:        body.Temperatura,
		FrequenciaCardiaca: body.FrequenciaCardiaca,
		SaturacaoO2:        body.SaturacaoO2,
		Peso:               body.Peso,
		Altura:             body.Altura,
		Sintomas:           body.Sintomas,
		Prioridade:         body.Prioridade,
		Observacoes:        body.Observacoes,
	}); err != nil {
		erro500(c, err)
		return
	}

	// Só avança quem ainda aguardava triagem; senão é uma correção.
	etapa := pg.Etapa
	campos := map[string]any{}
	if etapa == model.EtapaAguardandoTriagem {
		h.registrar(c, pg, model.MovTriagem, descreverTriagem("Triagem:", body))
	} else {
		h.registrar(c, pg, model.MovCorrecaoTriagem, descreverTriagem("Triagem corrigida:", body))
	}
	if etapa == model.EtapaAguardandoTriagem {
		etapa = model.EtapaAguardandoConsulta
		campos["etapa"] = etapa
		n, _ := h.passagens.CountByEtapa(etapa)
		campos["pos_x"], campos["pos_y"] = model.PosicaoPadrao(etapa, n)
	}
	if body.PosX != nil && body.PosY != nil {
		campos["pos_x"], campos["pos_y"] = model.ClampPosicao(etapa, *body.PosX, *body.PosY)
	}
	if len(campos) > 0 {
		if err := h.passagens.Update(pg.ID, campos); err != nil {
			erro500(c, err)
			return
		}
	}
	h.responder(c, pg.ID, http.StatusOK)
}

// @Summary         Registrar consulta e dar alta
// @Description     Salva a consulta e encerra a passagem: o card sai do quadro e vai para "Atendidos hoje". Exige triagem.
// @Tags            Passagens
// @Accept          json
// @Produce         json
// @Security        BearerAuth
// @Param           id        path  int              true  "ID da passagem"
// @Param           consulta  body  dto.ConsultaDTO  true  "Consulta"
// @Success         200  {object}  model.Passagem
// @Failure         400  {object}  map[string]string
// @Failure         401  {object}  map[string]string
// @Failure         404  {object}  map[string]string
// @Failure         409  {object}  map[string]string
// @Router          /passagens/{id}/consulta [post]
func (h *PassagemHandler) Consulta(c *gin.Context) {
	pg, ok := h.carregarAberta(c)
	if !ok {
		return
	}
	if pg.Triagem == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "O paciente precisa passar pela triagem antes da consulta"})
		return
	}
	var body dto.ConsultaDTO
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.passagens.UpsertConsulta(model.Consulta{
		PassagemID:        pg.ID,
		AutorID:           middleware.CurrentUserID(c),
		Anamnese:          body.Anamnese,
		Diagnostico:       body.Diagnostico,
		Prescricao:        body.Prescricao,
		ExamesSolicitados: body.ExamesSolicitados,
		Encaminhamento:    body.Encaminhamento,
		Retorno:           body.Retorno,
		Observacoes:       body.Observacoes,
	}); err != nil {
		erro500(c, err)
		return
	}
	agora := time.Now()
	if err := h.passagens.Update(pg.ID, map[string]any{"etapa": model.EtapaAlta, "alta_em": &agora}); err != nil {
		erro500(c, err)
		return
	}
	h.registrar(c, pg, model.MovConsulta, descreverConsulta(body))
	h.responder(c, pg.ID, http.StatusOK)
}
