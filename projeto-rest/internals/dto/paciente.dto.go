package dto

import (
	"time"

	"projeto-rest/internals/model"
)

// PacienteDTO é o corpo para criar ou atualizar um paciente. O REST repassa
// ao serviço SOAP, que é quem grava.
type PacienteDTO struct {
	Nome   string `json:"nome" binding:"required" example:"Ana Maria"`
	Status string `json:"status" example:"em observação"`
}

// ComentarioMovimentacao é o comentário do doutor sobre uma movimentação.
type ComentarioMovimentacao struct {
	MovimentacaoID uint   `json:"movimentacao_id"`
	Comentario     string `json:"comentario"`
}

// FinalizarDTO é o corpo para finalizar o plantão. Os comentários são opcionais.
type FinalizarDTO struct {
	Comentario  string                   `json:"comentario"`
	Comentarios []ComentarioMovimentacao `json:"comentarios"`
}

// PreviaFinalizacao é o resumo mostrado antes de finalizar: tudo o que o
// doutor registrou desde a última finalização.
type PreviaFinalizacao struct {
	Inicio time.Time `json:"inicio"`
	Fim    time.Time `json:"fim"`
	// Quantas movimentações de cada tipo (chegada, triagem, consulta...)
	Totais        map[string]int       `json:"totais"`
	Pacientes     int                  `json:"pacientes"`
	Movimentacoes []model.Movimentacao `json:"movimentacoes"`
}
