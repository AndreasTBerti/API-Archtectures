package dto

// Posicao é opcional na triagem: quando o doutor arrasta o card para a coluna
// "Aguardando consulta" e salva a triagem, o card fica onde foi solto.
type Posicao struct {
	PosX *float64 `json:"pos_x"`
	PosY *float64 `json:"pos_y"`
}

// AdmissaoDTO é o registro da chegada. Enviado para abrir a passagem do dia
// ou para corrigir a admissão de uma passagem aberta.
type AdmissaoDTO struct {
	QueixaPrincipal string `json:"queixa_principal" binding:"required"`
	FormaChegada    string `json:"forma_chegada" binding:"required,oneof=andando cadeira_rodas maca ambulancia" example:"andando"`
	Acompanhante    string `json:"acompanhante"`
	Convenio        string `json:"convenio"`
	Observacoes     string `json:"observacoes"`
}

// TriagemDTO é o formulário da triagem.
type TriagemDTO struct {
	PressaoArterial    string  `json:"pressao_arterial" binding:"required" example:"120/80"`
	Temperatura        float64 `json:"temperatura" binding:"required" example:"36.8"`
	FrequenciaCardiaca int     `json:"frequencia_cardiaca" example:"72"`
	SaturacaoO2        int     `json:"saturacao_o2" example:"98"`
	Peso               float64 `json:"peso" example:"70.5"`
	Altura             float64 `json:"altura" example:"1.75"`
	Sintomas           string  `json:"sintomas" binding:"required"`
	Prioridade         string  `json:"prioridade" binding:"required,oneof=baixa media alta emergencia" example:"media"`
	Observacoes        string  `json:"observacoes"`
	Posicao
}

// ConsultaDTO é o formulário da consulta. Salvá-la dá alta ao paciente.
type ConsultaDTO struct {
	Anamnese          string `json:"anamnese" binding:"required"`
	Diagnostico       string `json:"diagnostico" binding:"required"`
	Prescricao        string `json:"prescricao"`
	ExamesSolicitados string `json:"exames_solicitados"`
	Encaminhamento    string `json:"encaminhamento"`
	Retorno           string `json:"retorno" example:"2026-10-05"`
	Observacoes       string `json:"observacoes"`
}

// ResumoPassagem é o que importa de uma passagem anterior para o resumo.
type ResumoPassagem struct {
	PassagemID        uint    `json:"passagem_id"`
	Data              string  `json:"data" example:"2026-09-10"`
	QueixaPrincipal   string  `json:"queixa_principal,omitempty"`
	Prioridade        string  `json:"prioridade,omitempty"`
	PressaoArterial   string  `json:"pressao_arterial,omitempty"`
	Temperatura       float64 `json:"temperatura,omitempty"`
	SaturacaoO2       int     `json:"saturacao_o2,omitempty"`
	Peso              float64 `json:"peso,omitempty"`
	Sintomas          string  `json:"sintomas,omitempty"`
	Diagnostico       string  `json:"diagnostico,omitempty"`
	Prescricao        string  `json:"prescricao,omitempty"`
	ExamesSolicitados string  `json:"exames_solicitados,omitempty"`
	Encaminhamento    string  `json:"encaminhamento,omitempty"`
	Retorno           string  `json:"retorno,omitempty"`
}

// ResumoResponse é o resumo do paciente mostrado aos doutores. Usa só as
// passagens encerradas de dias anteriores, nunca a passagem atual.
type ResumoResponse struct {
	PacienteID uint `json:"paciente_id"`
	// Quantas passagens anteriores entraram no resumo
	TotalPassagens int    `json:"total_passagens"`
	UltimaVinda    string `json:"ultima_vinda,omitempty" example:"2026-09-10"`
	// Texto corrido do resumo e quem o gerou ("regras" hoje; a IA no futuro)
	Texto     string           `json:"texto"`
	GeradoPor string           `json:"gerado_por" example:"regras"`
	Passagens []ResumoPassagem `json:"passagens"`
}
