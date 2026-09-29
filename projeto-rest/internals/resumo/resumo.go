// Package resumo monta o resumo do paciente a partir das passagens
// anteriores. O texto vem de um Gerador: hoje um gerador por regras; depois,
// um gerador com IA que recebe exatamente os mesmos dados.
package resumo

import (
	"fmt"
	"strings"
	"time"

	"projeto-rest/internals/dto"
	"projeto-rest/internals/model"
)

// Gerador escreve o texto do resumo a partir das passagens anteriores
// (da mais recente para a mais antiga). Para trocar pela IA, basta outra
// implementação desta interface.
type Gerador interface {
	Nome() string
	Gerar(p model.Paciente, anteriores []dto.ResumoPassagem) (string, error)
}

// Montar converte as passagens anteriores e pede o texto ao gerador. Se o
// gerador falhar, o resumo sai sem texto, mas com os dados estruturados.
func Montar(g Gerador, p model.Paciente, anteriores []model.Passagem) dto.ResumoResponse {
	itens := make([]dto.ResumoPassagem, 0, len(anteriores))
	for _, pg := range anteriores {
		itens = append(itens, converter(pg))
	}
	r := dto.ResumoResponse{
		PacienteID:     p.ID,
		TotalPassagens: len(itens),
		GeradoPor:      g.Nome(),
		Passagens:      itens,
	}
	if len(itens) > 0 {
		r.UltimaVinda = itens[0].Data
	}
	if texto, err := g.Gerar(p, itens); err == nil {
		r.Texto = texto
	}
	return r
}

func converter(pg model.Passagem) dto.ResumoPassagem {
	r := dto.ResumoPassagem{PassagemID: pg.ID, Data: pg.Data}
	if a := pg.Admissao; a != nil {
		r.QueixaPrincipal = a.QueixaPrincipal
	}
	if t := pg.Triagem; t != nil {
		r.Prioridade = t.Prioridade
		r.PressaoArterial = t.PressaoArterial
		r.Temperatura = t.Temperatura
		r.SaturacaoO2 = t.SaturacaoO2
		r.Peso = t.Peso
		r.Sintomas = t.Sintomas
	}
	if c := pg.Consulta; c != nil {
		r.Diagnostico = c.Diagnostico
		r.Prescricao = c.Prescricao
		r.ExamesSolicitados = c.ExamesSolicitados
		r.Encaminhamento = c.Encaminhamento
		r.Retorno = c.Retorno
	}
	return r
}

// Regras é o gerador provisório, sem IA: monta frases fixas com os dados.
type Regras struct{}

func (Regras) Nome() string { return "regras" }

func (Regras) Gerar(_ model.Paciente, anteriores []dto.ResumoPassagem) (string, error) {
	if len(anteriores) == 0 {
		return "Primeira vinda registrada neste hospital. Não há passagens anteriores.", nil
	}
	var b strings.Builder
	ultima := anteriores[0]
	if len(anteriores) == 1 {
		fmt.Fprintf(&b, "Uma passagem anterior, em %s.", dataBR(ultima.Data))
	} else {
		fmt.Fprintf(&b, "%d passagens anteriores. A última foi em %s.", len(anteriores), dataBR(ultima.Data))
	}
	if ultima.QueixaPrincipal != "" {
		fmt.Fprintf(&b, " Veio com: %s.", semPonto(ultima.QueixaPrincipal))
	}
	if ultima.Diagnostico != "" {
		fmt.Fprintf(&b, " Diagnóstico: %s.", semPonto(ultima.Diagnostico))
	}
	if ultima.Prescricao != "" {
		fmt.Fprintf(&b, " Prescrição: %s.", semPonto(ultima.Prescricao))
	}
	if ultima.Retorno != "" {
		fmt.Fprintf(&b, " Retorno indicado: %s.", dataBR(ultima.Retorno))
	}

	// Diagnósticos das vindas mais antigas, sem repetir
	var outros []string
	visto := map[string]bool{strings.ToLower(ultima.Diagnostico): true}
	for _, a := range anteriores[1:] {
		d := strings.TrimSpace(a.Diagnostico)
		if d == "" || visto[strings.ToLower(d)] {
			continue
		}
		visto[strings.ToLower(d)] = true
		outros = append(outros, fmt.Sprintf("%s (%s)", semPonto(d), dataBR(a.Data)))
	}
	if len(outros) > 0 {
		fmt.Fprintf(&b, " Diagnósticos anteriores: %s.", strings.Join(outros, "; "))
	}

	graves := 0
	for _, a := range anteriores {
		if a.Prioridade == "alta" || a.Prioridade == "emergencia" {
			graves++
		}
	}
	if graves > 0 {
		fmt.Fprintf(&b, " Classificado como prioridade alta ou emergência em %d vinda(s).", graves)
	}
	return b.String(), nil
}

func dataBR(s string) string {
	if t, err := time.Parse(model.FormatoData, s); err == nil {
		return t.Format("02/01/2006")
	}
	return s
}

func semPonto(s string) string { return strings.TrimRight(strings.TrimSpace(s), ".") }
