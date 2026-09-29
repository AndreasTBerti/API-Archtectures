package model

import "time"

// Documentos de uma passagem. Cada passagem tem no máximo um de cada.
// AutorID guarda quem registrou o documento só para auditoria: não é um
// campo do formulário nem aparece no quadro.

// Admissao é o registro da chegada do paciente. Criá-la abre a passagem.
type Admissao struct {
	ID         uint `gorm:"primaryKey;autoincrement" json:"id"`
	PassagemID uint `gorm:"uniqueIndex;not null" json:"passagem_id"`
	AutorID    uint `gorm:"index" json:"autor_id"`

	QueixaPrincipal string `gorm:"type:text" json:"queixa_principal"`
	FormaChegada    string `gorm:"type:varchar(30)" json:"forma_chegada"` // andando, cadeira_rodas, maca, ambulancia
	Acompanhante    string `gorm:"type:varchar(100)" json:"acompanhante"`
	Convenio        string `gorm:"type:varchar(100)" json:"convenio"`
	Observacoes     string `gorm:"type:text" json:"observacoes"`

	CreatedAt time.Time `json:"registrada_em"`
	UpdatedAt time.Time `json:"atualizada_em"`
}

func (Admissao) TableName() string { return "admissoes" }

// Triagem leva a passagem de aguardando_triagem para aguardando_consulta.
type Triagem struct {
	ID         uint `gorm:"primaryKey;autoincrement" json:"id"`
	PassagemID uint `gorm:"uniqueIndex;not null" json:"passagem_id"`
	AutorID    uint `gorm:"index" json:"autor_id"`

	PressaoArterial    string  `gorm:"type:varchar(20)" json:"pressao_arterial"`
	Temperatura        float64 `json:"temperatura"`
	FrequenciaCardiaca int     `json:"frequencia_cardiaca"`
	SaturacaoO2        int     `json:"saturacao_o2"`
	Peso               float64 `json:"peso"`
	Altura             float64 `json:"altura"`
	Sintomas           string  `gorm:"type:text" json:"sintomas"`
	Prioridade         string  `gorm:"type:varchar(20)" json:"prioridade"` // baixa, media, alta, emergencia
	Observacoes        string  `gorm:"type:text" json:"observacoes"`

	CreatedAt time.Time `json:"registrada_em"`
	UpdatedAt time.Time `json:"atualizada_em"`
}

// Consulta encerra a passagem (alta): o card sai do quadro.
type Consulta struct {
	ID         uint `gorm:"primaryKey;autoincrement" json:"id"`
	PassagemID uint `gorm:"uniqueIndex;not null" json:"passagem_id"`
	AutorID    uint `gorm:"index" json:"autor_id"`

	Anamnese          string `gorm:"type:text" json:"anamnese"`
	Diagnostico       string `gorm:"type:text" json:"diagnostico"`
	Prescricao        string `gorm:"type:text" json:"prescricao"`
	ExamesSolicitados string `gorm:"type:text" json:"exames_solicitados"`
	Encaminhamento    string `gorm:"type:varchar(100)" json:"encaminhamento"`
	Retorno           string `gorm:"type:varchar(50)" json:"retorno"`
	Observacoes       string `gorm:"type:text" json:"observacoes"`

	CreatedAt time.Time `json:"registrada_em"`
	UpdatedAt time.Time `json:"atualizada_em"`
}
