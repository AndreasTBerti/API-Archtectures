package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"projeto-rest/internals/auth"
	"projeto-rest/internals/model"
	"projeto-rest/internals/soap"
)

// Apoio dos testes de integração: sobe a aplicação inteira (mesmo router do
// main) num servidor HTTP de teste, com um SQLite temporário e o SOAP falso.

const segredoTeste = "segredo-dos-testes"

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	gin.DefaultWriter = io.Discard // sem o log de cada requisição
	os.Exit(m.Run())
}

type app struct {
	t    *testing.T
	srv  *httptest.Server
	db   *gorm.DB
	soap *soapFalso
}

func novoApp(t *testing.T) *app {
	t.Helper()
	falso := novoSoapFalso(t)
	t.Setenv("SOAP_URL", falso.url())
	t.Setenv("JWT_SECRET", segredoTeste)
	t.Setenv("JWT_TTL_MINUTES", "")

	db, err := abrirBanco(filepath.Join(t.TempDir(), "rest.db"))
	if err != nil {
		t.Fatal("abrindo o banco:", err)
	}
	t.Cleanup(func() {
		// No Windows o arquivo precisa estar fechado para o TempDir ser apagado.
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})

	srv := httptest.NewServer(montarRouter(db, auth.NewFromEnv(), soap.NewFromEnv()))
	t.Cleanup(srv.Close)
	return &app{t: t, srv: srv, db: db, soap: falso}
}

// ---- HTTP ----

type resposta struct {
	status int
	header http.Header
	corpo  []byte
}

func (a *app) enviar(req *http.Request, token string) resposta {
	a.t.Helper()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close()
	corpo, _ := io.ReadAll(res.Body)
	return resposta{status: res.StatusCode, header: res.Header, corpo: corpo}
}

// chamar faz uma requisição JSON. corpo nil não envia corpo.
func (a *app) chamar(metodo, rota, token string, corpo any) resposta {
	a.t.Helper()
	var leitor io.Reader
	if corpo != nil {
		b, err := json.Marshal(corpo)
		if err != nil {
			a.t.Fatal(err)
		}
		leitor = bytes.NewReader(b)
	}
	req, err := http.NewRequest(metodo, a.srv.URL+rota, leitor)
	if err != nil {
		a.t.Fatal(err)
	}
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return a.enviar(req, token)
}

// formulario faz um POST application/x-www-form-urlencoded (fluxo OAuth2).
func (a *app) formulario(rota string, campos url.Values) resposta {
	a.t.Helper()
	req, err := http.NewRequest(http.MethodPost, a.srv.URL+rota, strings.NewReader(campos.Encode()))
	if err != nil {
		a.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return a.enviar(req, "")
}

// exigir falha o teste se o status não for o esperado, mostrando o corpo.
func exigir(t *testing.T, r resposta, status int) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status = %d, esperado %d; corpo: %s", r.status, status, r.corpo)
	}
}

// ler decodifica o corpo JSON da resposta.
func ler[T any](t *testing.T, r resposta) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(r.corpo, &v); err != nil {
		t.Fatalf("corpo não é o JSON esperado (%v): %s", err, r.corpo)
	}
	return v
}

// erroDe devolve o campo "error" do corpo.
func erroDe(t *testing.T, r resposta) string {
	t.Helper()
	return ler[map[string]any](t, r)["error"].(string)
}

// ---- fixtures ----

// doutor registra um doutor e devolve o token e o ID dele.
func (a *app) doutor(nome string) (string, uint) {
	a.t.Helper()
	r := a.chamar(http.MethodPost, "/auth/register", "", map[string]string{"nome": nome, "senha": "senha123"})
	exigir(a.t, r, http.StatusCreated)
	u := ler[model.User](a.t, r)
	return a.login(nome, "senha123"), u.ID
}

func (a *app) login(nome, senha string) string {
	a.t.Helper()
	r := a.chamar(http.MethodPost, "/auth/token", "", map[string]string{"nome": nome, "senha": senha})
	exigir(a.t, r, http.StatusOK)
	return ler[map[string]any](a.t, r)["access_token"].(string)
}

func (a *app) criarPaciente(token, nome string) model.Paciente {
	a.t.Helper()
	r := a.chamar(http.MethodPost, "/pacientes", token, map[string]string{"nome": nome, "status": "em observação"})
	exigir(a.t, r, http.StatusCreated)
	return ler[model.Paciente](a.t, r)
}

func admissao() map[string]any {
	return map[string]any{
		"queixa_principal": "Dor de cabeça forte",
		"forma_chegada":    "andando",
		"acompanhante":     "Filha",
		"convenio":         "SUS",
	}
}

func triagem() map[string]any {
	return map[string]any{
		"pressao_arterial":    "130/85",
		"temperatura":         38.2,
		"frequencia_cardiaca": 90,
		"saturacao_o2":        97,
		"peso":                70.5,
		"altura":              1.72,
		"sintomas":            "Febre e dor no corpo",
		"prioridade":          "alta",
	}
}

func consulta() map[string]any {
	return map[string]any{
		"anamnese":       "Febre há dois dias",
		"diagnostico":    "Virose",
		"prescricao":     "Dipirona 500mg",
		"encaminhamento": "",
		"retorno":        "2026-10-05",
	}
}

func (a *app) chegada(token string, pacienteID uint) model.Passagem {
	a.t.Helper()
	r := a.chamar(http.MethodPost, rotaPaciente(pacienteID, "/passagens"), token, admissao())
	exigir(a.t, r, http.StatusCreated)
	return ler[model.Passagem](a.t, r)
}

// atender leva a passagem até a alta (triagem e consulta).
func (a *app) atender(token string, passagemID uint) model.Passagem {
	a.t.Helper()
	exigir(a.t, a.chamar(http.MethodPost, rotaPassagem(passagemID, "/triagem"), token, triagem()), http.StatusOK)
	r := a.chamar(http.MethodPost, rotaPassagem(passagemID, "/consulta"), token, consulta())
	exigir(a.t, r, http.StatusOK)
	return ler[model.Passagem](a.t, r)
}

// passagemAntiga grava direto no banco uma vinda encerrada de dias atrás,
// com os três documentos. É o que alimenta o resumo do paciente.
func (a *app) passagemAntiga(pacienteID uint, diasAtras int, queixa, prioridade, diagnostico string) model.Passagem {
	a.t.Helper()
	chegada := time.Now().AddDate(0, 0, -diasAtras)
	alta := chegada.Add(2 * time.Hour)
	pg := model.Passagem{
		PacienteID: pacienteID, Data: model.DataDe(chegada), Etapa: model.EtapaAlta,
		ChegadaEm: chegada, AltaEm: &alta,
	}
	a.gravar(&pg)
	a.gravar(&model.Admissao{PassagemID: pg.ID, QueixaPrincipal: queixa, FormaChegada: "andando"})
	a.gravar(&model.Triagem{PassagemID: pg.ID, PressaoArterial: "120/80", Temperatura: 37, Prioridade: prioridade, Sintomas: queixa})
	a.gravar(&model.Consulta{PassagemID: pg.ID, Anamnese: "-", Diagnostico: diagnostico, Prescricao: "Repouso"})
	return pg
}

func (a *app) gravar(v any) {
	a.t.Helper()
	if err := a.db.Create(v).Error; err != nil {
		a.t.Fatal("gravando fixture:", err)
	}
}

func rotaPaciente(id uint, sufixo string) string {
	return "/pacientes/" + uintStr(id) + sufixo
}

func rotaPassagem(id uint, sufixo string) string {
	return "/passagens/" + uintStr(id) + sufixo
}

func uintStr(v uint) string { return strconv.FormatUint(uint64(v), 10) }

// esperarProximoSegundo: o iat do JWT tem precisão de segundos, e o middleware
// só recusa tokens emitidos num segundo anterior à finalização do plantão.
func esperarProximoSegundo() {
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second + 20*time.Millisecond)))
}

// ---- WebSocket ----

type mensagem map[string]any

type clienteWS struct {
	t    *testing.T
	conn *websocket.Conn
}

func (a *app) conectar(token string) *clienteWS {
	a.t.Helper()
	endereco := "ws" + strings.TrimPrefix(a.srv.URL, "http") + "/ws?token=" + url.QueryEscape(token)
	conn, res, err := websocket.DefaultDialer.Dial(endereco, nil)
	if err != nil {
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		a.t.Fatalf("conectando no WebSocket (HTTP %d): %v", status, err)
	}
	a.t.Cleanup(func() { conn.Close() })
	return &clienteWS{t: a.t, conn: conn}
}

func (c *clienteWS) enviar(msg map[string]any) {
	c.t.Helper()
	if err := c.conn.WriteJSON(msg); err != nil {
		c.t.Fatal("enviando pelo WebSocket:", err)
	}
}

// esperar lê mensagens até chegar uma do tipo pedido (as outras são ignoradas).
func (c *clienteWS) esperar(tipo string) mensagem {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		var m mensagem
		if err := c.conn.ReadJSON(&m); err != nil {
			c.t.Fatalf("esperando a mensagem %q: %v", tipo, err)
		}
		if m["type"] == tipo {
			return m
		}
	}
}

// esperarPassagem espera um passagem_updated da passagem informada.
func (c *clienteWS) esperarPassagem(id uint) map[string]any {
	c.t.Helper()
	for {
		p := c.esperar("passagem_updated")["passagem"].(map[string]any)
		if uint(p["id"].(float64)) == id {
			return p
		}
	}
}

// esperarFechamento espera o servidor fechar a conexão.
func (c *clienteWS) esperarFechamento() {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				c.t.Fatal("o servidor não fechou a conexão")
			}
			return
		}
	}
}
