// Package soap é um cliente mínimo para o ProntuarioService (projeto-soap).
// Monta o envelope SOAP 1.1 à mão e faz o parse da resposta com encoding/xml,
// sem depender de bibliotecas externas.
//
// O serviço SOAP é o dono dos dados do paciente (cadastro, consulta,
// alteração e remoção). O REST não guarda pacientes: usa este cliente.
package soap

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Prontuario espelha o ProntuarioModel do contrato SOAP (contrato_servico.wsdl).
type Prontuario struct {
	IDPaciente   int    `xml:"id_paciente" json:"id_paciente"`
	Paciente     string `xml:"paciente" json:"paciente"`
	Status       string `xml:"status" json:"status"`
	CriadoEm     string `xml:"criado_em" json:"criado_em"`
	AtualizadoEm string `xml:"atualizado_em" json:"atualizado_em"`
	MensagemErro string `xml:"mensagem_erro,omitempty" json:"mensagem_erro,omitempty"`
}

var (
	// ErrNaoEncontrado é devolvido quando o serviço responde com mensagem_erro.
	ErrNaoEncontrado = errors.New("paciente não encontrado no serviço SOAP")
	// ErrIndisponivel indica que o serviço SOAP não respondeu ou respondeu algo inválido.
	ErrIndisponivel = errors.New("serviço SOAP indisponível")
)

// ErroValidacao é um SOAP Fault do cliente (faultcode Client.*): dado inválido.
type ErroValidacao struct{ Mensagem string }

func (e *ErroValidacao) Error() string { return e.Mensagem }

type Client struct {
	url  string
	http *http.Client
}

// NewFromEnv lê SOAP_URL (padrão http://127.0.0.1:8000/).
func NewFromEnv() *Client {
	url := os.Getenv("SOAP_URL")
	if url == "" {
		url = "http://127.0.0.1:8000/"
	}
	return &Client{url: url, http: &http.Client{Timeout: 10 * time.Second}}
}

// resultado é a resposta das operações que devolvem um ProntuarioModel.
type resultado struct {
	Result Prontuario `xml:",any"`
}

// Estruturas de parse da resposta. O encoding/xml casa pelo nome local quando
// a tag não especifica namespace, então o prefixo (tns:, s0:) não importa.
type envelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    struct {
		GetPaciente       *resultado `xml:"get_pacienteResponse"`
		CriarPaciente     *resultado `xml:"criar_pacienteResponse"`
		AtualizarPaciente *resultado `xml:"atualizar_pacienteResponse"`
		RemoverPaciente   *resultado `xml:"remover_pacienteResponse"`
		ListPacientes     *struct {
			Result struct {
				Items []Prontuario `xml:"ProntuarioModel"`
			} `xml:"list_pacientesResult"`
		} `xml:"list_pacientesResponse"`
		Fault *struct {
			Code   string `xml:"faultcode"`
			String string `xml:"faultstring"`
		} `xml:"Fault"`
	} `xml:"Body"`
}

func escapar(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (c *Client) call(action, body string) (*envelope, error) {
	payload := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cli="clinica.soap">` +
		`<soapenv:Header/><soapenv:Body>` + body + `</soapenv:Body></soapenv:Envelope>`

	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewBufferString(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"`+action+`"`)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIndisponivel, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrIndisponivel, err)
	}

	var env envelope
	if err := xml.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("%w: resposta inválida (HTTP %d): %v", ErrIndisponivel, resp.StatusCode, err)
	}
	if f := env.Body.Fault; f != nil {
		// faultcode vem com prefixo (ex.: "soap11env:Client.Validacao")
		if code := f.Code[strings.LastIndex(f.Code, ":")+1:]; strings.HasPrefix(code, "Client") {
			return nil, &ErroValidacao{Mensagem: f.String}
		}
		return nil, fmt.Errorf("%w: fault %s: %s", ErrIndisponivel, f.Code, f.String)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrIndisponivel, resp.StatusCode)
	}
	return &env, nil
}

// um extrai o prontuário de uma operação que devolve ProntuarioModel.
func um(r *resultado, op string) (*Prontuario, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: resposta sem %sResponse", ErrIndisponivel, op)
	}
	if r.Result.MensagemErro != "" {
		return nil, ErrNaoEncontrado
	}
	p := r.Result
	return &p, nil
}

// GetPaciente chama a operação get_paciente.
func (c *Client) GetPaciente(id int) (*Prontuario, error) {
	env, err := c.call("get_paciente",
		fmt.Sprintf(`<cli:get_paciente><cli:id_paciente>%d</cli:id_paciente></cli:get_paciente>`, id))
	if err != nil {
		return nil, err
	}
	return um(env.Body.GetPaciente, "get_paciente")
}

// ListPacientes chama a operação list_pacientes.
func (c *Client) ListPacientes() ([]Prontuario, error) {
	env, err := c.call("list_pacientes", `<cli:list_pacientes/>`)
	if err != nil {
		return nil, err
	}
	if env.Body.ListPacientes == nil {
		return nil, fmt.Errorf("%w: resposta sem list_pacientesResponse", ErrIndisponivel)
	}
	return env.Body.ListPacientes.Result.Items, nil
}

// CriarPaciente chama a operação criar_paciente.
func (c *Client) CriarPaciente(paciente, status string) (*Prontuario, error) {
	env, err := c.call("criar_paciente", fmt.Sprintf(
		`<cli:criar_paciente><cli:paciente>%s</cli:paciente><cli:status>%s</cli:status></cli:criar_paciente>`,
		escapar(paciente), escapar(status)))
	if err != nil {
		return nil, err
	}
	return um(env.Body.CriarPaciente, "criar_paciente")
}

// AtualizarPaciente chama a operação atualizar_paciente.
func (c *Client) AtualizarPaciente(id int, paciente, status string) (*Prontuario, error) {
	env, err := c.call("atualizar_paciente", fmt.Sprintf(
		`<cli:atualizar_paciente><cli:id_paciente>%d</cli:id_paciente><cli:paciente>%s</cli:paciente><cli:status>%s</cli:status></cli:atualizar_paciente>`,
		id, escapar(paciente), escapar(status)))
	if err != nil {
		return nil, err
	}
	return um(env.Body.AtualizarPaciente, "atualizar_paciente")
}

// RemoverPaciente chama a operação remover_paciente.
func (c *Client) RemoverPaciente(id int) (*Prontuario, error) {
	env, err := c.call("remover_paciente",
		fmt.Sprintf(`<cli:remover_paciente><cli:id_paciente>%d</cli:id_paciente></cli:remover_paciente>`, id))
	if err != nil {
		return nil, err
	}
	return um(env.Body.RemoverPaciente, "remover_paciente")
}
