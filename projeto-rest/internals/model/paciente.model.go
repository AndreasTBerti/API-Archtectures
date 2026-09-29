package model

// Paciente vem do serviço SOAP, que é o dono desses dados. O REST não tem
// tabela de pacientes: ID é o id_paciente do SOAP e é por ele que as
// passagens e os documentos se ligam ao paciente.
type Paciente struct {
	ID           uint   `json:"id"`
	Nome         string `json:"nome"`
	Status       string `json:"status"`
	CriadoEm     string `json:"criado_em,omitempty"`
	AtualizadoEm string `json:"atualizado_em,omitempty"`
}
