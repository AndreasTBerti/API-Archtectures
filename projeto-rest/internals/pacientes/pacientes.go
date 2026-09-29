// Package pacientes dá ao REST acesso aos pacientes, que moram no serviço
// SOAP. O REST não tem tabela de pacientes: guarda só as passagens e os
// documentos, ligados ao paciente pelo id_paciente do SOAP.
package pacientes

import (
	"errors"
	"log"

	"projeto-rest/internals/model"
	"projeto-rest/internals/soap"
)

var (
	ErrNaoEncontrado = soap.ErrNaoEncontrado
	ErrIndisponivel  = soap.ErrIndisponivel
)

// Diretorio é o que o REST precisa saber sobre pacientes.
type Diretorio interface {
	Listar() ([]model.Paciente, error)
	Buscar(id uint) (*model.Paciente, error)
	Criar(nome, status string) (*model.Paciente, error)
	Atualizar(id uint, nome, status string) (*model.Paciente, error)
	Remover(id uint) (*model.Paciente, error)
	// Anexar preenche o paciente de cada passagem. Se o SOAP não responder,
	// as passagens seguem sem o paciente (o quadro mostra só o número).
	Anexar(ps []model.Passagem)
}

type soapDiretorio struct{ soap *soap.Client }

func NewSoap(c *soap.Client) Diretorio { return &soapDiretorio{c} }

func converter(p soap.Prontuario) model.Paciente {
	return model.Paciente{
		ID:           uint(p.IDPaciente),
		Nome:         p.Paciente,
		Status:       p.Status,
		CriadoEm:     p.CriadoEm,
		AtualizadoEm: p.AtualizadoEm,
	}
}

func um(p *soap.Prontuario, err error) (*model.Paciente, error) {
	if err != nil {
		return nil, err
	}
	m := converter(*p)
	return &m, nil
}

func (d *soapDiretorio) Listar() ([]model.Paciente, error) {
	lista, err := d.soap.ListPacientes()
	if err != nil {
		return nil, err
	}
	out := make([]model.Paciente, 0, len(lista))
	for _, p := range lista {
		out = append(out, converter(p))
	}
	return out, nil
}

func (d *soapDiretorio) Buscar(id uint) (*model.Paciente, error) {
	return um(d.soap.GetPaciente(int(id)))
}

func (d *soapDiretorio) Criar(nome, status string) (*model.Paciente, error) {
	return um(d.soap.CriarPaciente(nome, status))
}

func (d *soapDiretorio) Atualizar(id uint, nome, status string) (*model.Paciente, error) {
	return um(d.soap.AtualizarPaciente(int(id), nome, status))
}

func (d *soapDiretorio) Remover(id uint) (*model.Paciente, error) {
	return um(d.soap.RemoverPaciente(int(id)))
}

func (d *soapDiretorio) Anexar(ps []model.Passagem) {
	if len(ps) == 0 {
		return
	}
	porID := map[uint]*model.Paciente{}
	if len(ps) == 1 {
		// Uma passagem só: busca direto, sem listar todos.
		if p, err := d.Buscar(ps[0].PacienteID); err == nil {
			porID[p.ID] = p
		} else if !errors.Is(err, ErrNaoEncontrado) {
			log.Println("pacientes: SOAP:", err)
		}
	} else {
		lista, err := d.Listar()
		if err != nil {
			log.Println("pacientes: SOAP:", err)
		}
		for i := range lista {
			porID[lista[i].ID] = &lista[i]
		}
	}
	for i := range ps {
		ps[i].Paciente = porID[ps[i].PacienteID]
	}
}

// AnexarUma é o atalho de Anexar para uma única passagem.
func AnexarUma(d Diretorio, p *model.Passagem) {
	ps := []model.Passagem{*p}
	d.Anexar(ps)
	p.Paciente = ps[0].Paciente
}
