package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// soapFalso imita o ProntuarioService (projeto-soap) para os testes de
// integração: guarda os pacientes em memória e responde com o mesmo XML que o
// Spyne gera, inclusive os SOAP Faults de validação. Assim o cliente SOAP real
// do REST (internals/soap) é exercitado sem precisar do Python.
type soapFalso struct {
	srv *httptest.Server

	mu        sync.Mutex
	pacientes map[int]pacienteFalso
	proximo   int
	foraDoAr  bool
	chamadas  map[string]int
}

type pacienteFalso struct {
	id                 int
	nome, status       string
	criado, atualizado string
}

func novoSoapFalso(t *testing.T) *soapFalso {
	t.Helper()
	agora := time.Now().UTC().Format(time.RFC3339)
	s := &soapFalso{
		// Mesmos pacientes iniciais do banco.py
		pacientes: map[int]pacienteFalso{
			1: {1, "José", "doente", agora, agora},
			2: {2, "Roberto", "saudável", agora, agora},
		},
		proximo:  3,
		chamadas: map[string]int{},
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.servir))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *soapFalso) url() string { return s.srv.URL + "/" }

// derrubar faz o serviço responder como se estivesse fora do ar.
func (s *soapFalso) derrubar(fora bool) {
	s.mu.Lock()
	s.foraDoAr = fora
	s.mu.Unlock()
}

func (s *soapFalso) totalChamadas(op string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chamadas[op]
}

func (s *soapFalso) existe(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.pacientes[id]
	return ok
}

// campos lê os elementos folha do corpo SOAP (nome local -> texto).
func campos(corpo []byte) map[string]string {
	out := map[string]string{}
	dec := xml.NewDecoder(strings.NewReader(string(corpo)))
	atual := ""
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		switch el := tok.(type) {
		case xml.StartElement:
			atual = el.Name.Local
			out[atual] = ""
		case xml.CharData:
			if atual != "" {
				out[atual] += string(el)
			}
		case xml.EndElement:
			atual = ""
		}
	}
}

func esc(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (p pacienteFalso) xml() string {
	return fmt.Sprintf(`<tns:id_paciente>%d</tns:id_paciente><tns:paciente>%s</tns:paciente>`+
		`<tns:status>%s</tns:status><tns:criado_em>%s</tns:criado_em><tns:atualizado_em>%s</tns:atualizado_em>`,
		p.id, esc(p.nome), esc(p.status), p.criado, p.atualizado)
}

const naoEncontradoXML = `<tns:mensagem_erro>Paciente não encontrado</tns:mensagem_erro>`

func envelopeSoap(corpo string) string {
	return `<?xml version='1.0' encoding='UTF-8'?>` +
		`<soap11env:Envelope xmlns:soap11env="http://schemas.xmlsoap.org/soap/envelope/" xmlns:tns="clinica.soap">` +
		`<soap11env:Body>` + corpo + `</soap11env:Body></soap11env:Envelope>`
}

func (s *soapFalso) servir(w http.ResponseWriter, r *http.Request) {
	corpo, _ := io.ReadAll(r.Body)
	op := strings.Trim(r.Header.Get("SOAPAction"), `"`)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.chamadas[op]++

	if s.foraDoAr {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	c := campos(corpo)
	id, _ := strconv.Atoi(c["id_paciente"])

	responder := func(resultado string) {
		fmt.Fprint(w, envelopeSoap(fmt.Sprintf(`<tns:%[1]sResponse><tns:%[1]sResult>%[2]s</tns:%[1]sResult></tns:%[1]sResponse>`, op, resultado)))
	}
	um := func(p pacienteFalso, ok bool) {
		if !ok {
			responder(naoEncontradoXML)
			return
		}
		responder(p.xml())
	}
	// Mesma validação do _validar() do server.py
	validar := func() (string, string, bool) {
		nome, status := strings.TrimSpace(c["paciente"]), strings.TrimSpace(c["status"])
		msg := ""
		switch {
		case nome == "":
			msg = "O nome do paciente é obrigatório"
		case len([]rune(nome)) > 100:
			msg = "O nome do paciente passa de 100 caracteres"
		case len([]rune(status)) > 50:
			msg = "O status passa de 50 caracteres"
		}
		if msg != "" {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, envelopeSoap(`<soap11env:Fault><faultcode>soap11env:Client.Validacao</faultcode>`+
				`<faultstring>`+esc(msg)+`</faultstring><faultactor></faultactor></soap11env:Fault>`))
			return "", "", false
		}
		return nome, status, true
	}
	agora := time.Now().UTC().Format(time.RFC3339)

	switch op {
	case "get_paciente":
		p, ok := s.pacientes[id]
		um(p, ok)

	case "list_pacientes":
		lista := make([]pacienteFalso, 0, len(s.pacientes))
		for _, p := range s.pacientes {
			lista = append(lista, p)
		}
		sort.Slice(lista, func(i, j int) bool {
			a, b := strings.ToLower(lista[i].nome), strings.ToLower(lista[j].nome)
			if a != b {
				return a < b
			}
			return lista[i].id < lista[j].id
		})
		var b strings.Builder
		for _, p := range lista {
			b.WriteString("<tns:ProntuarioModel>" + p.xml() + "</tns:ProntuarioModel>")
		}
		responder(b.String())

	case "criar_paciente":
		nome, status, ok := validar()
		if !ok {
			return
		}
		p := pacienteFalso{s.proximo, nome, status, agora, agora}
		s.pacientes[p.id] = p
		s.proximo++
		um(p, true)

	case "atualizar_paciente":
		nome, status, ok := validar()
		if !ok {
			return
		}
		p, ok := s.pacientes[id]
		if ok {
			p.nome, p.status, p.atualizado = nome, status, agora
			s.pacientes[id] = p
		}
		um(p, ok)

	case "remover_paciente":
		p, ok := s.pacientes[id]
		delete(s.pacientes, id)
		um(p, ok)

	default:
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, envelopeSoap(`<soap11env:Fault><faultcode>soap11env:Client.ResourceNotFound</faultcode>`+
			`<faultstring>operação desconhecida</faultstring></soap11env:Fault>`))
	}
}
