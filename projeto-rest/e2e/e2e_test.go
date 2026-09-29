//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ================================================================ ambiente

var (
	baseREST string
	baseSOAP string
	soapProc *processo
)

type processo struct {
	nome string
	cmd  *exec.Cmd
	log  string
	fim  chan struct{} // fechado quando o processo termina
}

func TestMain(m *testing.M) { os.Exit(executar(m)) }

func executar(m *testing.M) int {
	tmp, err := os.MkdirTemp("", "projeto-rest-e2e-*")
	if err != nil {
		fmt.Println("e2e:", err)
		return 1
	}
	defer os.RemoveAll(tmp)

	restDir, _ := filepath.Abs("..")
	soapDir := filepath.Join(restDir, "..", "projeto-soap")

	python, err := pythonDoSoap(soapDir)
	if err != nil {
		fmt.Println("e2e:", err)
		return 1
	}

	// 1. Serviço SOAP real, com banco temporário (começa com José e Roberto).
	portaSOAP := portaLivre()
	baseSOAP = fmt.Sprintf("http://127.0.0.1:%d/", portaSOAP)
	soapProc, err = iniciar(tmp, "soap", soapDir, []string{
		fmt.Sprintf("SOAP_PORT=%d", portaSOAP),
		"SOAP_DB=" + filepath.Join(tmp, "prontuarios.db"),
		"PYTHONUNBUFFERED=1",
	}, python, "server.py")
	if err != nil {
		fmt.Println("e2e:", err)
		return 1
	}
	defer soapProc.parar()
	if err := esperarPronto(baseSOAP+"?wsdl", soapProc); err != nil {
		fmt.Println("e2e:", err)
		return 1
	}

	// 2. Binário do REST, compilado do código atual.
	bin := filepath.Join(tmp, "projeto-rest")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = restDir
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Printf("e2e: go build falhou: %v\n%s", err, out)
		return 1
	}

	portaREST := portaLivre()
	baseREST = fmt.Sprintf("http://127.0.0.1:%d", portaREST)
	// Roda na pasta do projeto porque o quadro é servido de ./web/index.html.
	rest, err := iniciar(tmp, "rest", restDir, []string{
		fmt.Sprintf("PORT=%d", portaREST),
		"REST_DB=" + filepath.Join(tmp, "rest.db"),
		"SOAP_URL=" + baseSOAP,
		"JWT_SECRET=segredo-e2e",
		"GIN_MODE=release",
	}, bin)
	if err != nil {
		fmt.Println("e2e:", err)
		return 1
	}
	defer rest.parar()
	if err := esperarPronto(baseREST+"/health", rest); err != nil {
		fmt.Println("e2e:", err)
		return 1
	}

	codigo := m.Run()
	if codigo != 0 {
		rest.mostrarLog()
		soapProc.mostrarLog()
	}
	return codigo
}

// pythonDoSoap devolve o Python do ambiente do projeto-soap, criando o
// ambiente com "uv sync" se ele ainda não existir. O Python é chamado direto
// (e não via "uv run") para que parar o processo pare de fato o servidor.
func pythonDoSoap(dir string) (string, error) {
	candidatos := []string{filepath.Join(dir, ".venv", "Scripts", "python.exe"), filepath.Join(dir, ".venv", "bin", "python")}
	achar := func() string {
		for _, c := range candidatos {
			if _, err := os.Stat(c); err == nil {
				return c
			}
		}
		return ""
	}
	if p := achar(); p != "" {
		return p, nil
	}
	if _, err := exec.LookPath("uv"); err != nil {
		return "", errors.New("o ambiente do projeto-soap não existe e o uv não está instalado (rode `uv sync` em projeto-soap)")
	}
	sync := exec.Command("uv", "sync")
	sync.Dir = dir
	if out, err := sync.CombinedOutput(); err != nil {
		return "", fmt.Errorf("uv sync falhou: %v\n%s", err, out)
	}
	if p := achar(); p != "" {
		return p, nil
	}
	return "", errors.New("uv sync não criou o .venv do projeto-soap")
}

func portaLivre() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func iniciar(tmp, nome, dir string, env []string, prog string, args ...string) (*processo, error) {
	logPath := filepath.Join(tmp, nome+".log")
	arq, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(prog, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout, cmd.Stderr = arq, arq
	if err := cmd.Start(); err != nil {
		arq.Close()
		return nil, fmt.Errorf("iniciando %s: %w", nome, err)
	}
	p := &processo{nome: nome, cmd: cmd, log: logPath, fim: make(chan struct{})}
	go func() { cmd.Wait(); arq.Close(); close(p.fim) }()
	return p, nil
}

func (p *processo) parar() {
	p.cmd.Process.Kill()
	// espera o processo sair e liberar o log antes do RemoveAll
	select {
	case <-p.fim:
	case <-time.After(5 * time.Second):
	}
}

func (p *processo) mostrarLog() {
	b, _ := os.ReadFile(p.log)
	if len(b) > 4000 {
		b = b[len(b)-4000:]
	}
	fmt.Printf("\n----- log do %s (final) -----\n%s\n", p.nome, b)
}

func esperarPronto(endereco string, p *processo) error {
	limite := time.Now().Add(30 * time.Second)
	for time.Now().Before(limite) {
		select {
		case <-p.fim:
			p.mostrarLog()
			return fmt.Errorf("%s encerrou antes de ficar pronto", p.nome)
		default:
		}
		if res, err := http.Get(endereco); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	p.mostrarLog()
	return fmt.Errorf("%s não respondeu em %s", p.nome, endereco)
}

// ================================================================ cliente

type resposta struct {
	status int
	header http.Header
	corpo  []byte
}

func (r resposta) texto() string { return string(r.corpo) }

func requisitar(t *testing.T, req *http.Request, token string) resposta {
	t.Helper()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer res.Body.Close()
	corpo, _ := io.ReadAll(res.Body)
	return resposta{res.StatusCode, res.Header, corpo}
}

func chamar(t *testing.T, metodo, rota, token string, corpo any) resposta {
	t.Helper()
	var leitor io.Reader
	if corpo != nil {
		b, _ := json.Marshal(corpo)
		leitor = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(metodo, baseREST+rota, leitor)
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return requisitar(t, req, token)
}

func exigir(t *testing.T, r resposta, status int) {
	t.Helper()
	if r.status != status {
		t.Fatalf("status = %d, esperado %d; corpo: %s", r.status, status, r.corpo)
	}
}

type obj = map[string]any

func ler[T any](t *testing.T, r resposta) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(r.corpo, &v); err != nil {
		t.Fatalf("corpo não é o JSON esperado (%v): %s", err, r.corpo)
	}
	return v
}

func id(v any) int { return int(v.(float64)) }

// unico gera nomes diferentes a cada execução (o SOAP é compartilhado entre os testes).
func unico(prefixo string) string { return fmt.Sprintf("%s-%d", prefixo, time.Now().UnixNano()) }

// doutor registra um doutor e faz login pelo fluxo OAuth2 (form-urlencoded).
func doutor(t *testing.T, nome string) string {
	t.Helper()
	exigir(t, chamar(t, http.MethodPost, "/auth/register", "", obj{"nome": nome, "senha": "senha123"}), http.StatusCreated)
	return login(t, nome)
}

func login(t *testing.T, nome string) string {
	t.Helper()
	form := url.Values{"grant_type": {"password"}, "username": {nome}, "password": {"senha123"}}
	req, _ := http.NewRequest(http.MethodPost, baseREST+"/auth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r := requisitar(t, req, "")
	exigir(t, r, http.StatusOK)
	tok := ler[obj](t, r)
	if tok["token_type"] != "Bearer" {
		t.Fatalf("token_type = %v", tok["token_type"])
	}
	return tok["access_token"].(string)
}

func criarPaciente(t *testing.T, token, nome string) int {
	t.Helper()
	r := chamar(t, http.MethodPost, "/pacientes", token, obj{"nome": nome, "status": "em observação"})
	exigir(t, r, http.StatusCreated)
	return id(ler[obj](t, r)["id"])
}

var (
	admissao = obj{"queixa_principal": "Dor abdominal", "forma_chegada": "ambulancia", "convenio": "SUS"}
	triagem  = obj{"pressao_arterial": "140/90", "temperatura": 37.9, "saturacao_o2": 96, "sintomas": "Dor e náusea", "prioridade": "alta"}
	consulta = obj{"anamnese": "Dor há 6 horas", "diagnostico": "Gastrite", "prescricao": "Omeprazol", "retorno": "2026-10-10"}
)

func chegada(t *testing.T, token string, paciente int) obj {
	t.Helper()
	r := chamar(t, http.MethodPost, fmt.Sprintf("/pacientes/%d/passagens", paciente), token, admissao)
	exigir(t, r, http.StatusCreated)
	return ler[obj](t, r)
}

func documento(t *testing.T, token string, passagem int, tipo string, corpo obj) obj {
	t.Helper()
	r := chamar(t, http.MethodPost, fmt.Sprintf("/passagens/%d/%s", passagem, tipo), token, corpo)
	exigir(t, r, http.StatusOK)
	return ler[obj](t, r)
}

// soapGetPaciente chama o serviço SOAP diretamente, sem passar pelo REST.
func soapGetPaciente(t *testing.T, paciente int) string {
	t.Helper()
	envelope := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>`+
		`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cli="clinica.soap">`+
		`<soapenv:Body><cli:get_paciente><cli:id_paciente>%d</cli:id_paciente></cli:get_paciente></soapenv:Body></soapenv:Envelope>`, paciente)
	req, _ := http.NewRequest(http.MethodPost, baseSOAP, strings.NewReader(envelope))
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", `"get_paciente"`)
	return requisitar(t, req, "").texto()
}

// ---- WebSocket ----

type ws struct {
	t    *testing.T
	conn *websocket.Conn
}

func conectar(t *testing.T, token string) *ws {
	t.Helper()
	endereco := "ws" + strings.TrimPrefix(baseREST, "http") + "/ws?token=" + url.QueryEscape(token)
	conn, _, err := websocket.DefaultDialer.Dial(endereco, nil)
	if err != nil {
		t.Fatal("conectando no quadro:", err)
	}
	t.Cleanup(func() { conn.Close() })
	c := &ws{t, conn}
	c.esperar("init")
	return c
}

func (c *ws) enviar(m obj) {
	c.t.Helper()
	if err := c.conn.WriteJSON(m); err != nil {
		c.t.Fatal(err)
	}
}

func (c *ws) esperar(tipo string) obj {
	c.t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var m obj
		if err := c.conn.ReadJSON(&m); err != nil {
			c.t.Fatalf("esperando %q no quadro: %v", tipo, err)
		}
		if m["type"] == tipo {
			return m
		}
	}
}

func (c *ws) esperarPassagem(passagem int) obj {
	c.t.Helper()
	for {
		if p := c.esperar("passagem_updated")["passagem"].(obj); id(p["id"]) == passagem {
			return p
		}
	}
}

// esperarEtapa pula os avisos antigos da passagem até ela chegar na etapa.
func (c *ws) esperarEtapa(passagem int, etapa string) obj {
	c.t.Helper()
	for {
		if p := c.esperarPassagem(passagem); p["etapa"] == etapa {
			return p
		}
	}
}

// ================================================================ testes
//
// Os testes compartilham os dois servidores e rodam na ordem do arquivo. O de
// SOAP fora do ar derruba o serviço e por isso fica por último.

func TestE2E_PaginasPublicas(t *testing.T) {
	exigir(t, chamar(t, http.MethodGet, "/health", "", nil), http.StatusOK)

	r := chamar(t, http.MethodGet, "/", "", nil)
	exigir(t, r, http.StatusOK)
	if !strings.Contains(r.texto(), "<canvas") {
		t.Error("a página do quadro não tem o canvas")
	}

	r = chamar(t, http.MethodGet, "/swagger/doc.json", "", nil)
	exigir(t, r, http.StatusOK)
	for _, rota := range []string{"/auth/token", "/pacientes/{id}/passagens", "/finalizacao"} {
		if !strings.Contains(r.texto(), rota) {
			t.Errorf("a documentação Swagger não tem %s", rota)
		}
	}
}

func TestE2E_PacientesFicamNoSoapReal(t *testing.T) {
	token := doutor(t, unico("dra.ana"))

	r := chamar(t, http.MethodGet, "/pacientes", token, nil)
	exigir(t, r, http.StatusOK)
	nomes := []string{}
	for _, p := range ler[[]obj](t, r) {
		nomes = append(nomes, p["nome"].(string))
	}
	if !strings.Contains(strings.Join(nomes, ","), "José") || !strings.Contains(strings.Join(nomes, ","), "Roberto") {
		t.Errorf("pacientes iniciais do SOAP não vieram: %v", nomes)
	}

	nome := unico("Maria & Filhos")
	pid := criarPaciente(t, token, nome)

	// O cadastro feito pelo REST está no banco do SOAP.
	if xml := soapGetPaciente(t, pid); !strings.Contains(xml, "Maria &amp; Filhos") {
		t.Fatalf("o SOAP não tem o paciente criado pelo REST: %s", xml)
	}

	r = chamar(t, http.MethodPut, fmt.Sprintf("/pacientes/%d", pid), token, obj{"nome": "Maria Silva", "status": "estável"})
	exigir(t, r, http.StatusOK)
	if xml := soapGetPaciente(t, pid); !strings.Contains(xml, "Maria Silva") || !strings.Contains(xml, "estável") {
		t.Errorf("a alteração não chegou no SOAP: %s", xml)
	}

	// A validação é do SOAP: o Fault Client.Validacao vira 400 com a mensagem dele.
	r = chamar(t, http.MethodPost, "/pacientes", token, obj{"nome": strings.Repeat("a", 101)})
	exigir(t, r, http.StatusBadRequest)
	if e := ler[obj](t, r)["error"]; e != "O nome do paciente passa de 100 caracteres" {
		t.Errorf("error = %v", e)
	}

	exigir(t, chamar(t, http.MethodGet, "/pacientes/999999", token, nil), http.StatusNotFound)
}

func TestE2E_JornadaDoPacienteComDoisDoutores(t *testing.T) {
	tokenA := doutor(t, unico("dra.bia"))
	tokenB := doutor(t, unico("dr.caio"))
	nome := unico("Paciente Jornada")
	pid := criarPaciente(t, tokenA, nome)

	quadroA, quadroB := conectar(t, tokenA), conectar(t, tokenB)

	// A registra a chegada; B vê o card aparecer com o nome vindo do SOAP.
	pg := chegada(t, tokenA, pid)
	pgID := id(pg["id"])
	card := quadroB.esperarPassagem(pgID)
	if card["etapa"] != "aguardando_triagem" || card["paciente"].(obj)["nome"] != nome {
		t.Fatalf("card no quadro de B = %v", card)
	}
	exigir(t, chamar(t, http.MethodPost, fmt.Sprintf("/pacientes/%d/passagens", pid), tokenB, admissao), http.StatusConflict)

	// B arrasta o card: A vê e não consegue pegá-lo ao mesmo tempo.
	quadroB.enviar(obj{"type": "drag_start", "passagem_id": pgID})
	quadroA.esperar("drag_start")
	quadroA.enviar(obj{"type": "drag_start", "passagem_id": pgID})
	quadroA.esperar("drag_denied")
	quadroB.enviar(obj{"type": "drag_end", "passagem_id": pgID, "x": 400, "y": 300})
	if p := quadroA.esperarPassagem(pgID); p["pos_x"].(float64) != 400 {
		t.Errorf("A não viu o card na posição nova: %v", p["pos_x"])
	}

	// B faz a triagem; A vê o card mudar de coluna.
	documento(t, tokenB, pgID, "triagem", triagem)
	quadroA.esperarEtapa(pgID, "aguardando_consulta")

	// A faz a consulta: alta.
	alta := documento(t, tokenA, pgID, "consulta", consulta)
	if alta["etapa"] != "alta" || alta["alta_em"] == nil {
		t.Fatalf("depois da consulta = %v", alta)
	}
	quadroB.esperarEtapa(pgID, "alta")

	// O quadro REST lista a passagem em "Atendidos hoje", com os três documentos.
	r := chamar(t, http.MethodGet, "/quadro", tokenB, nil)
	exigir(t, r, http.StatusOK)
	var achou obj
	for _, p := range ler[[]obj](t, r) {
		if id(p["id"]) == pgID {
			achou = p
		}
	}
	if achou == nil || achou["admissao"] == nil || achou["triagem"] == nil || achou["consulta"] == nil {
		t.Fatalf("passagem no /quadro = %v", achou)
	}
	if autor := id(achou["triagem"].(obj)["autor_id"]); autor == id(achou["consulta"].(obj)["autor_id"]) {
		t.Error("triagem e consulta foram feitas por doutores diferentes, mas têm o mesmo autor_id")
	}

	// Histórico e resumo: a vinda de hoje não entra no resumo.
	r = chamar(t, http.MethodGet, fmt.Sprintf("/pacientes/%d/passagens", pid), tokenA, nil)
	exigir(t, r, http.StatusOK)
	if n := len(ler[[]obj](t, r)); n != 1 {
		t.Errorf("histórico com %d passagens, esperado 1", n)
	}
	r = chamar(t, http.MethodGet, fmt.Sprintf("/pacientes/%d/resumo", pid), tokenA, nil)
	exigir(t, r, http.StatusOK)
	if res := ler[obj](t, r); res["total_passagens"].(float64) != 0 || !strings.Contains(res["texto"].(string), "Primeira vinda") {
		t.Errorf("resumo = %v", res)
	}

	// Volta no mesmo dia: a passagem do dia é reaberta.
	r = chamar(t, http.MethodPost, fmt.Sprintf("/pacientes/%d/passagens", pid), tokenA, admissao)
	exigir(t, r, http.StatusOK)
	if p := ler[obj](t, r); id(p["id"]) != pgID || p["etapa"] != "aguardando_triagem" {
		t.Errorf("passagem reaberta = %v", p)
	}
}

func TestE2E_RemoverPaciente(t *testing.T) {
	token := doutor(t, unico("dra.dora"))
	pid := criarPaciente(t, token, unico("Paciente Removido"))
	quadro := conectar(t, token)

	pgID := id(chegada(t, token, pid)["id"])
	exigir(t, chamar(t, http.MethodDelete, fmt.Sprintf("/pacientes/%d", pid), token, nil), http.StatusConflict)
	if xml := soapGetPaciente(t, pid); strings.Contains(xml, "mensagem_erro") {
		t.Fatal("o paciente sumiu do SOAP mesmo estando no hospital")
	}

	documento(t, token, pgID, "triagem", triagem)
	documento(t, token, pgID, "consulta", consulta)
	exigir(t, chamar(t, http.MethodDelete, fmt.Sprintf("/pacientes/%d", pid), token, nil), http.StatusOK)

	if m := quadro.esperar("paciente_deleted"); id(m["paciente_id"]) != pid {
		t.Errorf("paciente_deleted = %v", m)
	}
	if xml := soapGetPaciente(t, pid); !strings.Contains(xml, "Paciente não encontrado") {
		t.Errorf("o paciente continua no SOAP: %s", xml)
	}
	exigir(t, chamar(t, http.MethodGet, fmt.Sprintf("/pacientes/%d", pid), token, nil), http.StatusNotFound)
	exigir(t, chamar(t, http.MethodGet, fmt.Sprintf("/passagens/%d", pgID), token, nil), http.StatusNotFound)
}

func TestE2E_FinalizarPlantao(t *testing.T) {
	nome := unico("dr.edu")
	token := doutor(t, nome)
	colega := doutor(t, unico("dra.fabi"))
	pid := criarPaciente(t, token, unico("Paciente Plantão"))
	quadro := conectar(t, token)

	pgID := id(chegada(t, token, pid)["id"])
	documento(t, token, pgID, "triagem", triagem)

	r := chamar(t, http.MethodGet, "/finalizacao", token, nil)
	exigir(t, r, http.StatusOK)
	previa := ler[obj](t, r)
	movs := previa["movimentacoes"].([]any)
	if len(movs) != 2 || previa["totais"].(obj)["triagem"].(float64) != 1 {
		t.Fatalf("prévia = %v", previa)
	}
	triagemID := id(movs[1].(obj)["id"])

	// O iat do JWT tem precisão de segundos: finaliza num segundo depois do login.
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second + 20*time.Millisecond)))

	r = chamar(t, http.MethodPost, "/finalizacao", token, obj{
		"comentario":  "Plantão com uma internação",
		"comentarios": []obj{{"movimentacao_id": triagemID, "comentario": "Pedir endoscopia"}},
	})
	exigir(t, r, http.StatusCreated)

	quadro.esperar("sessao_encerrada")
	exigir(t, chamar(t, http.MethodGet, "/auth/me", token, nil), http.StatusUnauthorized)

	// O colega vê o comentário na passagem.
	r = chamar(t, http.MethodGet, fmt.Sprintf("/passagens/%d", pgID), colega, nil)
	exigir(t, r, http.StatusOK)
	cs := ler[obj](t, r)["comentarios"].([]any)
	if len(cs) != 1 || cs[0].(obj)["comentario"] != "Pedir endoscopia" {
		t.Errorf("comentários da passagem = %v", cs)
	}

	// Novo login: plantão novo, com a finalização no histórico.
	novo := login(t, nome)
	r = chamar(t, http.MethodGet, "/finalizacoes", novo, nil)
	exigir(t, r, http.StatusOK)
	if fs := ler[[]obj](t, r); len(fs) != 1 || fs[0]["comentario"] != "Plantão com uma internação" {
		t.Errorf("finalizações = %v", fs)
	}
}

// Fica por último: derruba o serviço SOAP.
func TestE2E_SoapForaDoAr(t *testing.T) {
	token := doutor(t, unico("dr.gil"))
	pid := criarPaciente(t, token, unico("Paciente Sem SOAP"))
	chegada(t, token, pid)

	soapProc.parar()

	exigir(t, chamar(t, http.MethodGet, "/health", "", nil), http.StatusOK)
	exigir(t, chamar(t, http.MethodGet, "/pacientes", token, nil), http.StatusBadGateway)
	exigir(t, chamar(t, http.MethodGet, fmt.Sprintf("/pacientes/%d", pid), token, nil), http.StatusBadGateway)

	// O quadro continua funcionando com os dados do REST, só sem os nomes.
	r := chamar(t, http.MethodGet, "/quadro", token, nil)
	exigir(t, r, http.StatusOK)
	ps := ler[[]obj](t, r)
	if len(ps) == 0 {
		t.Fatal("o quadro ficou vazio sem o SOAP")
	}
	for _, p := range ps {
		if p["paciente"] != nil {
			t.Errorf("passagem %v com paciente sem o SOAP no ar", p["id"])
		}
	}
}
