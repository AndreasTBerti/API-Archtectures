package model

import "time"

// Tipos de movimentação registrados enquanto o doutor trabalha.
const (
	MovChegada          = "chegada"
	MovNovaChegada      = "nova_chegada" // voltou no mesmo dia depois da alta
	MovTriagem          = "triagem"
	MovConsulta         = "consulta" // consulta e alta
	MovCorrecaoAdmissao = "correcao_admissao"
	MovCorrecaoTriagem  = "correcao_triagem"
)

// Movimentacao é uma ação feita por um doutor numa passagem. Enquanto
// FinalizacaoID é nulo, ela está pendente e aparece no resumo do plantão.
// Ao finalizar, o doutor pode deixar um comentário em cada uma.
type Movimentacao struct {
	ID            uint      `gorm:"primaryKey;autoincrement" json:"id"`
	DoutorID      uint      `gorm:"not null;index" json:"doutor_id"`
	PassagemID    uint      `gorm:"not null;index" json:"passagem_id"`
	PacienteID    uint      `gorm:"not null;index" json:"paciente_id"`
	PacienteNome  string    `gorm:"type:varchar(100)" json:"paciente_nome"`
	Tipo          string    `gorm:"type:varchar(30);not null" json:"tipo" example:"triagem"`
	Descricao     string    `gorm:"type:text" json:"descricao"`
	Comentario    string    `gorm:"type:text" json:"comentario"`
	FinalizacaoID *uint     `gorm:"index" json:"finalizacao_id"`
	CreatedAt     time.Time `json:"em"`
}

func (Movimentacao) TableName() string { return "movimentacoes" }

// Finalizacao é o fechamento do plantão de um doutor: junta as movimentações
// pendentes dele, com o comentário geral e os comentários de cada uma.
// Depois de finalizar, a sessão do doutor é encerrada.
type Finalizacao struct {
	ID            uint           `gorm:"primaryKey;autoincrement" json:"id"`
	DoutorID      uint           `gorm:"not null;index" json:"doutor_id"`
	Inicio        time.Time      `json:"inicio"`
	Fim           time.Time      `json:"fim"`
	Comentario    string         `gorm:"type:text" json:"comentario"`
	Movimentacoes []Movimentacao `gorm:"foreignKey:FinalizacaoID" json:"movimentacoes"`
}

func (Finalizacao) TableName() string { return "finalizacoes" }
