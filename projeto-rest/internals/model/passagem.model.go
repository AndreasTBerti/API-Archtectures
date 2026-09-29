package model

import "time"

// Etapas de uma passagem. As duas primeiras são as colunas do quadro, e cada
// coluna diz o que o paciente está esperando. EtapaAlta encerra a passagem:
// o card sai do quadro e vai para "Atendidos hoje".
const (
	EtapaAguardandoTriagem  = "aguardando_triagem"  // depois da chegada (admissão)
	EtapaAguardandoConsulta = "aguardando_consulta" // depois da triagem
	EtapaAlta               = "alta"                // depois da consulta, fora do quadro
)

// Etapas são as colunas do quadro, na ordem. EtapaAlta fica de fora de propósito.
var Etapas = []string{EtapaAguardandoTriagem, EtapaAguardandoConsulta}

// EtapaIndex devolve a coluna da etapa no quadro, ou -1 se não for coluna.
func EtapaIndex(etapa string) int {
	for i, e := range Etapas {
		if e == etapa {
			return i
		}
	}
	return -1
}

// FormatoData é o formato do dia de uma passagem (AAAA-MM-DD).
const FormatoData = "2006-01-02"

// DataDe devolve o dia (horário local) de um instante.
func DataDe(t time.Time) string { return t.In(time.Local).Format(FormatoData) }

// Hoje devolve o dia atual no horário local.
func Hoje() string { return DataDe(time.Now()) }

// Passagem é uma vinda do paciente ao hospital. Existe no máximo uma por
// paciente por dia (o dia da chegada). Se o paciente volta no mesmo dia
// depois da alta, a passagem do dia é reaberta.
type Passagem struct {
	ID uint `gorm:"primaryKey;autoincrement" json:"id"`
	// PacienteID é o id_paciente do serviço SOAP (o REST não guarda pacientes).
	PacienteID uint `gorm:"not null;uniqueIndex:idx_passagem_paciente_data,priority:1" json:"paciente_id"`
	// Paciente é preenchido a partir do SOAP na leitura; não é gravado aqui.
	Paciente *Paciente `gorm:"-" json:"paciente,omitempty"`
	Data     string    `gorm:"type:varchar(10);not null;uniqueIndex:idx_passagem_paciente_data,priority:2" json:"data" example:"2026-09-22"`
	Etapa    string    `gorm:"type:varchar(30);not null;index" json:"etapa" example:"aguardando_triagem"`

	// Posição do card no quadro (coordenadas lógicas, canto superior esquerdo)
	PosX float64 `gorm:"not null;default:0" json:"pos_x"`
	PosY float64 `gorm:"not null;default:0" json:"pos_y"`

	ChegadaEm time.Time  `json:"chegada_em"`
	AltaEm    *time.Time `json:"alta_em"`

	// Documentos desta passagem (nulos enquanto não preenchidos)
	Admissao *Admissao `gorm:"foreignKey:PassagemID" json:"admissao"`
	Triagem  *Triagem  `gorm:"foreignKey:PassagemID" json:"triagem"`
	Consulta *Consulta `gorm:"foreignKey:PassagemID" json:"consulta"`

	// Comentários que os doutores deixaram sobre esta passagem ao finalizar o plantão
	Comentarios []Movimentacao `gorm:"foreignKey:PassagemID" json:"comentarios"`

	// Quantas passagens o paciente teve antes desta (calculado na leitura)
	Anteriores int `gorm:"-" json:"passagens_anteriores"`

	CreatedAt time.Time `json:"criada_em"`
	UpdatedAt time.Time `json:"atualizada_em"`
}

func (Passagem) TableName() string { return "passagens" }

// Aberta indica se o paciente ainda está no hospital nesta passagem.
func (p *Passagem) Aberta() bool { return p.AltaEm == nil }

// Geometria lógica do quadro, compartilhada com o frontend (web/index.html).
var (
	QuadroLargura = 1800.0
	QuadroAltura  = 900.0
	ColunaLargura = QuadroLargura / float64(len(Etapas))
	CardLargura   = 220.0
	CardAltura    = 96.0
)

// ColunaDe devolve em qual coluna do quadro está a coordenada x
// (o quadro usa o centro do card para decidir a coluna).
func ColunaDe(x float64) int {
	col := int(x / ColunaLargura)
	if col < 0 {
		return 0
	}
	if col > len(Etapas)-1 {
		return len(Etapas) - 1
	}
	return col
}

// ClampPosicao mantém o card inteiro dentro da coluna da etapa.
func ClampPosicao(etapa string, x, y float64) (float64, float64) {
	col := EtapaIndex(etapa)
	if col < 0 {
		col = 0
	}
	const margem = 10.0
	const topo = 70.0 // altura do cabeçalho da coluna
	minX := float64(col)*ColunaLargura + margem
	maxX := float64(col+1)*ColunaLargura - CardLargura - margem
	x = clamp(x, minX, maxX)
	y = clamp(y, topo, QuadroAltura-CardAltura-margem)
	return x, y
}

// PosicaoPadrao é onde um card novo entra na coluna: empilha em fileiras.
func PosicaoPadrao(etapa string, n int64) (float64, float64) {
	const porFileira = 7
	linha := n % porFileira
	fileira := (n / porFileira) % 3
	return ClampPosicao(etapa,
		float64(EtapaIndex(etapa))*ColunaLargura+20+float64(fileira)*(CardLargura+20),
		80+float64(linha)*(CardAltura+10))
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
