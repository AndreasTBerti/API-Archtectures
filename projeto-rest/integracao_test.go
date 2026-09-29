package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"projeto-rest/internals/auth"
	"projeto-rest/internals/dto"
	"projeto-rest/internals/model"
)

// Testes de integração: cada teste sobe a aplicação inteira (router, middleware
// JWT, handlers, repositórios, SQLite e cliente SOAP) com um banco novo e o
// SOAP falso de soapfalso_test.go, e conversa com ela por HTTP e WebSocket.

// ---------------------------------------------------------------- sistema

func TestHealthEPingSaoPublicos(t *testing.T) {
	a := novoApp(t)
	for _, rota := range []string{"/health", "/ping"} {
		r := a.chamar(http.MethodGet, rota, "", nil)
		exigir(t, r, http.StatusOK)
		if msg := ler[map[string]string](t, r)["message"]; msg != "pong" {
			t.Errorf("%s: message = %q", rota, msg)
		}
	}
}

func TestQuadroWebEhServidoNaRaiz(t *testing.T) {
	a := novoApp(t)
	r := a.chamar(http.MethodGet, "/", "", nil)
	exigir(t, r, http.StatusOK)
	if !strings.Contains(r.header.Get("Content-Type"), "text/html") {
		t.Errorf("Content-Type = %q", r.header.Get("Content-Type"))
	}
}

// ---------------------------------------------------------------- autenticação

func TestRegistroELoginComJSON(t *testing.T) {
	a := novoApp(t)

	r := a.chamar(http.MethodPost, "/auth/register", "", map[string]string{"nome": "dra.ana", "senha": "123456"})
	exigir(t, r, http.StatusCreated)
	if strings.Contains(string(r.corpo), "123456") || strings.Contains(string(r.corpo), "hash") {
		t.Errorf("a resposta do registro expõe a senha: %s", r.corpo)
	}
	criado := ler[model.User](t, r)

	r = a.chamar(http.MethodPost, "/auth/token", "", map[string]string{"nome": "dra.ana", "senha": "123456"})
	exigir(t, r, http.StatusOK)
	tok := ler[dto.TokenResponse](t, r)
	if tok.TokenType != "Bearer" || tok.ExpiresIn != 3600 || tok.AccessToken == "" {
		t.Fatalf("token inesperado: %+v", tok)
	}

	r = a.chamar(http.MethodGet, "/auth/me", tok.AccessToken, nil)
	exigir(t, r, http.StatusOK)
	if eu := ler[model.User](t, r); eu.ID != criado.ID || eu.Nome != "dra.ana" {
		t.Errorf("/auth/me = %+v, esperado o doutor %d", eu, criado.ID)
	}
}

func TestLoginNoFormatoOAuth2(t *testing.T) {
	a := novoApp(t)
	a.doutor("dr.bruno")

	r := a.formulario("/auth/token", url.Values{
		"grant_type": {"password"}, "username": {"dr.bruno"}, "password": {"senha123"},
	})
	exigir(t, r, http.StatusOK)
	token := ler[dto.TokenResponse](t, r).AccessToken
	exigir(t, a.chamar(http.MethodGet, "/auth/me", token, nil), http.StatusOK)

	r = a.formulario("/auth/token", url.Values{
		"grant_type": {"client_credentials"}, "username": {"dr.bruno"}, "password": {"senha123"},
	})
	exigir(t, r, http.StatusBadRequest)
	if e := ler[dto.OAuthError](t, r); e.Error != "unsupported_grant_type" {
		t.Errorf("error = %q, esperado unsupported_grant_type", e.Error)
	}
}

func TestErrosDeAutenticacao(t *testing.T) {
	a := novoApp(t)
	a.doutor("dra.carla")

	casos := []struct {
		nome   string
		rota   string
		corpo  map[string]string
		status int
		erro   string
	}{
		{"registro sem senha", "/auth/register", map[string]string{"nome": "x"}, http.StatusBadRequest, ""},
		{"registro duplicado", "/auth/register", map[string]string{"nome": "dra.carla", "senha": "outra"}, http.StatusConflict, "Usuário já existe"},
		{"login sem senha", "/auth/token", map[string]string{"nome": "dra.carla"}, http.StatusBadRequest, "invalid_request"},
		{"senha errada", "/auth/token", map[string]string{"nome": "dra.carla", "senha": "errada"}, http.StatusUnauthorized, "invalid_grant"},
		{"doutor inexistente", "/auth/token", map[string]string{"nome": "ninguem", "senha": "senha123"}, http.StatusUnauthorized, "invalid_grant"},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			r := a.chamar(http.MethodPost, c.rota, "", c.corpo)
			exigir(t, r, c.status)
			if c.erro != "" && erroDe(t, r) != c.erro {
				t.Errorf("error = %q, esperado %q", erroDe(t, r), c.erro)
			}
		})
	}
}

func assinar(t *testing.T, segredo string, claims auth.Claims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(segredo))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMiddlewareJWT(t *testing.T) {
	a := novoApp(t)
	token, id := a.doutor("dr.davi")

	claims := func(emitido time.Time, validade time.Duration) auth.Claims {
		return auth.Claims{UserID: id, Nome: "dr.davi", RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "projeto-rest",
			IssuedAt:  jwt.NewNumericDate(emitido),
			ExpiresAt: jwt.NewNumericDate(emitido.Add(validade)),
		}}
	}
	expirado := assinar(t, segredoTeste, claims(time.Now().Add(-2*time.Hour), time.Hour))
	outroSegredo := assinar(t, "outro-segredo", claims(time.Now(), time.Hour))

	casos := []struct {
		nome   string
		header string
		query  string
		status int
	}{
		{"sem token", "", "", http.StatusUnauthorized},
		{"Bearer válido", "Bearer " + token, "", http.StatusOK},
		{"token puro (Swagger)", token, "", http.StatusOK},
		{"token na query string (WebSocket)", "", "?token=" + token, http.StatusOK},
		{"esquema Basic", "Basic " + token, "", http.StatusUnauthorized},
		{"token malformado", "Bearer abc.def.ghi", "", http.StatusUnauthorized},
		{"token expirado", "Bearer " + expirado, "", http.StatusUnauthorized},
		{"assinado com outro segredo", "Bearer " + outroSegredo, "", http.StatusUnauthorized},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, a.srv.URL+"/auth/me"+c.query, nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			r := a.enviar(req, "")
			exigir(t, r, c.status)
			if c.status == http.StatusUnauthorized && c.header == "" && r.header.Get("WWW-Authenticate") == "" {
				t.Error("401 sem o header WWW-Authenticate")
			}
		})
	}
}

func TestTodasAsRotasPrivadasExigemToken(t *testing.T) {
	a := novoApp(t)
	rotas := [][2]string{
		{"GET", "/auth/me"}, {"GET", "/quadro"}, {"GET", "/ws"},
		{"GET", "/users"}, {"POST", "/users"}, {"GET", "/users/1"}, {"PUT", "/users/1"}, {"DELETE", "/users/1"},
		{"GET", "/pacientes"}, {"POST", "/pacientes"}, {"GET", "/pacientes/1"}, {"PUT", "/pacientes/1"}, {"DELETE", "/pacientes/1"},
		{"GET", "/pacientes/1/passagens"}, {"POST", "/pacientes/1/passagens"}, {"GET", "/pacientes/1/resumo"},
		{"GET", "/passagens/1"}, {"POST", "/passagens/1/admissao"}, {"POST", "/passagens/1/triagem"}, {"POST", "/passagens/1/consulta"},
		{"GET", "/finalizacao"}, {"POST", "/finalizacao"}, {"GET", "/finalizacoes"},
	}
	for _, rt := range rotas {
		if r := a.chamar(rt[0], rt[1], "", nil); r.status != http.StatusUnauthorized {
			t.Errorf("%s %s sem token: status %d, esperado 401", rt[0], rt[1], r.status)
		}
	}
	if n := a.soap.totalChamadas("get_paciente") + a.soap.totalChamadas("list_pacientes"); n != 0 {
		t.Errorf("o SOAP foi chamado %d vez(es) sem autenticação", n)
	}
}

func TestTokenDeDoutorRemovido(t *testing.T) {
	a := novoApp(t)
	token, id := a.doutor("dr.eduardo")
	exigir(t, a.chamar(http.MethodDelete, "/users/"+uintStr(id), token, nil), http.StatusOK)

	r := a.chamar(http.MethodGet, "/auth/me", token, nil)
	exigir(t, r, http.StatusUnauthorized)
}

// ---------------------------------------------------------------- doutores

func TestCRUDDeDoutores(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("admin")

	r := a.chamar(http.MethodPost, "/users", token, map[string]string{"nome": "dra.fernanda", "senha": "abc123"})
	exigir(t, r, http.StatusCreated)
	nova := ler[model.User](t, r)

	exigir(t, a.chamar(http.MethodPost, "/users", token, map[string]string{"nome": "dra.fernanda", "senha": "x"}), http.StatusConflict)

	r = a.chamar(http.MethodGet, "/users", token, nil)
	exigir(t, r, http.StatusOK)
	if lista := ler[[]model.User](t, r); len(lista) != 2 {
		t.Fatalf("GET /users devolveu %d doutores, esperado 2", len(lista))
	}

	rota := "/users/" + uintStr(nova.ID)
	r = a.chamar(http.MethodGet, rota, token, nil)
	exigir(t, r, http.StatusOK)
	if u := ler[model.User](t, r); u.Nome != "dra.fernanda" {
		t.Errorf("nome = %q", u.Nome)
	}

	// Atualizar troca nome e senha: o login passa a ser com os dados novos.
	r = a.chamar(http.MethodPut, rota, token, map[string]string{"nome": "dra.fernanda.s", "senha": "nova-senha"})
	exigir(t, r, http.StatusOK)
	a.login("dra.fernanda.s", "nova-senha")
	exigir(t, a.chamar(http.MethodPost, "/auth/token", "", map[string]string{"nome": "dra.fernanda.s", "senha": "abc123"}), http.StatusUnauthorized)

	exigir(t, a.chamar(http.MethodDelete, rota, token, nil), http.StatusOK)
	exigir(t, a.chamar(http.MethodGet, rota, token, nil), http.StatusNotFound)
	exigir(t, a.chamar(http.MethodDelete, rota, token, nil), http.StatusNotFound)
	exigir(t, a.chamar(http.MethodPut, rota, token, map[string]string{"nome": "x", "senha": "y"}), http.StatusNotFound)
	exigir(t, a.chamar(http.MethodGet, "/users/abc", token, nil), http.StatusBadRequest)
	exigir(t, a.chamar(http.MethodPost, "/users", token, map[string]string{"nome": "sem-senha"}), http.StatusBadRequest)
}

// ---------------------------------------------------------------- pacientes (SOAP)

func TestPacientesSaoRepassadosAoSoap(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dra.gabi")

	r := a.chamar(http.MethodGet, "/pacientes", token, nil)
	exigir(t, r, http.StatusOK)
	lista := ler[[]model.Paciente](t, r)
	if len(lista) != 2 || lista[0].Nome != "José" || lista[1].Nome != "Roberto" {
		t.Fatalf("lista inicial = %+v, esperado José e Roberto em ordem alfabética", lista)
	}

	// Caracteres especiais atravessam o envelope XML sem se perder.
	p := a.criarPaciente(token, "Ana <Maria> & Cia")
	if p.ID != 3 || p.Nome != "Ana <Maria> & Cia" || p.Status != "em observação" || p.CriadoEm == "" {
		t.Fatalf("paciente criado = %+v", p)
	}

	r = a.chamar(http.MethodGet, rotaPaciente(p.ID, ""), token, nil)
	exigir(t, r, http.StatusOK)
	if got := ler[model.Paciente](t, r); got.Nome != p.Nome {
		t.Errorf("GET devolveu %+v", got)
	}

	r = a.chamar(http.MethodPut, rotaPaciente(p.ID, ""), token, map[string]string{"nome": "Ana Maria", "status": "estável"})
	exigir(t, r, http.StatusOK)
	if got := ler[model.Paciente](t, r); got.Nome != "Ana Maria" || got.Status != "estável" {
		t.Errorf("PUT devolveu %+v", got)
	}

	r = a.chamar(http.MethodDelete, rotaPaciente(p.ID, ""), token, nil)
	exigir(t, r, http.StatusOK)
	if a.soap.existe(int(p.ID)) {
		t.Error("o paciente continua no SOAP depois do DELETE")
	}
	exigir(t, a.chamar(http.MethodGet, rotaPaciente(p.ID, ""), token, nil), http.StatusNotFound)
}

func TestErrosDePacientes(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dr.heitor")

	t.Run("nome ausente é barrado no REST, sem chamar o SOAP", func(t *testing.T) {
		r := a.chamar(http.MethodPost, "/pacientes", token, map[string]string{"status": "x"})
		exigir(t, r, http.StatusBadRequest)
		if n := a.soap.totalChamadas("criar_paciente"); n != 0 {
			t.Errorf("criar_paciente foi chamado %d vez(es)", n)
		}
	})
	t.Run("Fault de validação do SOAP vira 400 com a mensagem dele", func(t *testing.T) {
		r := a.chamar(http.MethodPost, "/pacientes", token, map[string]string{"nome": strings.Repeat("a", 101)})
		exigir(t, r, http.StatusBadRequest)
		if e := erroDe(t, r); e != "O nome do paciente passa de 100 caracteres" {
			t.Errorf("error = %q", e)
		}
		r = a.chamar(http.MethodPut, "/pacientes/1", token, map[string]string{"nome": "   "})
		exigir(t, r, http.StatusBadRequest)
		if e := erroDe(t, r); e != "O nome do paciente é obrigatório" {
			t.Errorf("error = %q", e)
		}
	})
	t.Run("paciente inexistente vira 404", func(t *testing.T) {
		exigir(t, a.chamar(http.MethodGet, "/pacientes/999", token, nil), http.StatusNotFound)
		exigir(t, a.chamar(http.MethodPut, "/pacientes/999", token, map[string]string{"nome": "X"}), http.StatusNotFound)
		exigir(t, a.chamar(http.MethodDelete, "/pacientes/999", token, nil), http.StatusNotFound)
	})
	t.Run("ID inválido vira 400", func(t *testing.T) {
		exigir(t, a.chamar(http.MethodGet, "/pacientes/abc", token, nil), http.StatusBadRequest)
		exigir(t, a.chamar(http.MethodGet, "/pacientes/-1", token, nil), http.StatusBadRequest)
	})
}

func TestSoapForaDoArViraBadGateway(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dra.iara")
	a.soap.derrubar(true)

	exigir(t, a.chamar(http.MethodGet, "/pacientes", token, nil), http.StatusBadGateway)
	exigir(t, a.chamar(http.MethodGet, "/pacientes/1", token, nil), http.StatusBadGateway)
	exigir(t, a.chamar(http.MethodPost, "/pacientes", token, map[string]string{"nome": "X"}), http.StatusBadGateway)
	exigir(t, a.chamar(http.MethodPost, "/pacientes/1/passagens", token, admissao()), http.StatusBadGateway)

	a.soap.derrubar(false)
	exigir(t, a.chamar(http.MethodGet, "/pacientes", token, nil), http.StatusOK)
}

func TestNaoRemovePacienteQueEstaNoHospital(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dr.joao")
	pg := a.chegada(token, 1)

	r := a.chamar(http.MethodDelete, "/pacientes/1", token, nil)
	exigir(t, r, http.StatusConflict)
	if !a.soap.existe(1) || a.soap.totalChamadas("remover_paciente") != 0 {
		t.Fatal("o paciente foi removido do SOAP mesmo estando no hospital")
	}

	// Depois da alta pode remover, e as passagens e movimentações somem do REST.
	a.atender(token, pg.ID)
	exigir(t, a.chamar(http.MethodDelete, "/pacientes/1", token, nil), http.StatusOK)

	var passagens, docs, movs int64
	a.db.Model(&model.Passagem{}).Where("paciente_id = 1").Count(&passagens)
	a.db.Model(&model.Triagem{}).Where("passagem_id = ?", pg.ID).Count(&docs)
	a.db.Model(&model.Movimentacao{}).Where("paciente_id = 1").Count(&movs)
	if passagens+docs+movs != 0 {
		t.Errorf("sobrou no REST: %d passagem(ns), %d triagem(ns), %d movimentação(ões)", passagens, docs, movs)
	}
	exigir(t, a.chamar(http.MethodGet, rotaPassagem(pg.ID, ""), token, nil), http.StatusNotFound)
}

// ---------------------------------------------------------------- passagens

func TestFluxoCompletoDeUmaPassagem(t *testing.T) {
	a := novoApp(t)
	token, doutorID := a.doutor("dra.karen")

	// Chegada: abre a passagem de hoje em aguardando_triagem.
	pg := a.chegada(token, 1)
	if pg.Etapa != model.EtapaAguardandoTriagem || pg.Data != model.Hoje() || !pg.Aberta() {
		t.Fatalf("passagem aberta = %+v", pg)
	}
	if pg.Paciente == nil || pg.Paciente.Nome != "José" {
		t.Errorf("paciente não veio do SOAP: %+v", pg.Paciente)
	}
	if pg.Admissao == nil || pg.Admissao.QueixaPrincipal != "Dor de cabeça forte" || pg.Admissao.AutorID != doutorID {
		t.Errorf("admissão = %+v", pg.Admissao)
	}
	if pg.PosX >= model.ColunaLargura {
		t.Errorf("card fora da coluna de triagem: pos_x = %v", pg.PosX)
	}

	// Já está no hospital: não abre outra.
	r := a.chamar(http.MethodPost, "/pacientes/1/passagens", token, admissao())
	exigir(t, r, http.StatusConflict)
	if e := erroDe(t, r); e != "O paciente já está no hospital" {
		t.Errorf("error = %q", e)
	}

	// Consulta antes da triagem é recusada.
	exigir(t, a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/consulta"), token, consulta()), http.StatusConflict)

	// Triagem: vai para aguardando_consulta, na segunda coluna.
	r = a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/triagem"), token, triagem())
	exigir(t, r, http.StatusOK)
	pg = ler[model.Passagem](t, r)
	if pg.Etapa != model.EtapaAguardandoConsulta || pg.Triagem == nil || pg.Triagem.Prioridade != "alta" {
		t.Fatalf("depois da triagem: etapa %q, triagem %+v", pg.Etapa, pg.Triagem)
	}
	if pg.PosX < model.ColunaLargura {
		t.Errorf("card não mudou de coluna: pos_x = %v", pg.PosX)
	}

	// Reenviar a triagem corrige sem mudar a etapa; a posição pedida é
	// limitada à coluna da etapa.
	corr := triagem()
	corr["prioridade"], corr["pos_x"], corr["pos_y"] = "emergencia", 5.0, 5000.0
	r = a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/triagem"), token, corr)
	exigir(t, r, http.StatusOK)
	pg = ler[model.Passagem](t, r)
	x, y := model.ClampPosicao(model.EtapaAguardandoConsulta, 5, 5000)
	if pg.Etapa != model.EtapaAguardandoConsulta || pg.Triagem.Prioridade != "emergencia" || pg.PosX != x || pg.PosY != y {
		t.Errorf("correção da triagem: etapa %q, prioridade %q, pos (%v,%v), esperado (%v,%v)",
			pg.Etapa, pg.Triagem.Prioridade, pg.PosX, pg.PosY, x, y)
	}

	// Corrigir a admissão também não muda a etapa.
	adm := admissao()
	adm["queixa_principal"] = "Febre"
	r = a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/admissao"), token, adm)
	exigir(t, r, http.StatusOK)
	if pg = ler[model.Passagem](t, r); pg.Etapa != model.EtapaAguardandoConsulta || pg.Admissao.QueixaPrincipal != "Febre" {
		t.Errorf("correção da admissão: etapa %q, queixa %q", pg.Etapa, pg.Admissao.QueixaPrincipal)
	}

	// Consulta: alta.
	r = a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/consulta"), token, consulta())
	exigir(t, r, http.StatusOK)
	pg = ler[model.Passagem](t, r)
	if pg.Etapa != model.EtapaAlta || pg.AltaEm == nil || pg.Consulta == nil || pg.Consulta.Diagnostico != "Virose" {
		t.Fatalf("depois da consulta: %+v", pg)
	}

	// Passagem encerrada não aceita mais documentos.
	for _, doc := range []struct {
		rota  string
		corpo map[string]any
	}{{"/admissao", admissao()}, {"/triagem", triagem()}, {"/consulta", consulta()}} {
		r := a.chamar(http.MethodPost, rotaPassagem(pg.ID, doc.rota), token, doc.corpo)
		exigir(t, r, http.StatusConflict)
	}

	// GET da passagem traz os três documentos.
	r = a.chamar(http.MethodGet, rotaPassagem(pg.ID, ""), token, nil)
	exigir(t, r, http.StatusOK)
	if got := ler[model.Passagem](t, r); got.Admissao == nil || got.Triagem == nil || got.Consulta == nil {
		t.Errorf("documentos faltando: %+v", got)
	}
}

func TestValidacaoDosDocumentos(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dr.lucas")

	com := func(base map[string]any, campo string, valor any) map[string]any {
		if valor == nil {
			delete(base, campo)
		} else {
			base[campo] = valor
		}
		return base
	}

	casos := []struct {
		nome  string
		corpo map[string]any
	}{
		{"admissão sem queixa", com(admissao(), "queixa_principal", nil)},
		{"forma de chegada inválida", com(admissao(), "forma_chegada", "helicoptero")},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			exigir(t, a.chamar(http.MethodPost, "/pacientes/1/passagens", token, c.corpo), http.StatusBadRequest)
		})
	}
	var n int64
	a.db.Model(&model.Passagem{}).Count(&n)
	if n != 0 {
		t.Fatalf("admissão inválida abriu %d passagem(ns)", n)
	}

	pg := a.chegada(token, 1)
	casos = []struct {
		nome  string
		corpo map[string]any
	}{
		{"triagem sem pressão", com(triagem(), "pressao_arterial", nil)},
		{"triagem sem sintomas", com(triagem(), "sintomas", nil)},
		{"prioridade inválida", com(triagem(), "prioridade", "urgentissima")},
	}
	for _, c := range casos {
		t.Run(c.nome, func(t *testing.T) {
			exigir(t, a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/triagem"), token, c.corpo), http.StatusBadRequest)
		})
	}

	exigir(t, a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/triagem"), token, triagem()), http.StatusOK)
	exigir(t, a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/consulta"), token, com(consulta(), "diagnostico", nil)), http.StatusBadRequest)

	exigir(t, a.chamar(http.MethodPost, "/passagens/999/triagem", token, triagem()), http.StatusNotFound)
	exigir(t, a.chamar(http.MethodGet, "/passagens/999", token, nil), http.StatusNotFound)
	exigir(t, a.chamar(http.MethodPost, "/pacientes/999/passagens", token, admissao()), http.StatusNotFound)
}

func TestPacienteVoltaNoMesmoDiaDepoisDaAlta(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dra.marta")
	primeira := a.chegada(token, 2)
	a.atender(token, primeira.ID)

	r := a.chamar(http.MethodPost, "/pacientes/2/passagens", token, admissao())
	exigir(t, r, http.StatusOK) // reaberta, não criada
	pg := ler[model.Passagem](t, r)
	if pg.ID != primeira.ID || pg.Etapa != model.EtapaAguardandoTriagem || pg.AltaEm != nil {
		t.Fatalf("passagem reaberta = id %d, etapa %q, alta %v; esperado id %d em aguardando_triagem",
			pg.ID, pg.Etapa, pg.AltaEm, primeira.ID)
	}

	previa := ler[dto.PreviaFinalizacao](t, a.chamar(http.MethodGet, "/finalizacao", token, nil))
	if previa.Totais[model.MovNovaChegada] != 1 {
		t.Errorf("totais = %v, esperado uma nova_chegada", previa.Totais)
	}
}

func TestPassagemAbertaDeOutroDiaBloqueiaNovaChegada(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dr.nuno")
	ontem := time.Now().AddDate(0, 0, -1)
	a.gravar(&model.Passagem{PacienteID: 1, Data: model.DataDe(ontem), Etapa: model.EtapaAguardandoConsulta, ChegadaEm: ontem})

	r := a.chamar(http.MethodPost, "/pacientes/1/passagens", token, admissao())
	exigir(t, r, http.StatusConflict)
	if e := erroDe(t, r); !strings.Contains(e, ontem.Format("02/01/2006")) {
		t.Errorf("error = %q, esperado citar a data %s", e, ontem.Format("02/01/2006"))
	}
}

func TestQuadro(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dra.olga")

	antiga := a.passagemAntiga(1, 3, "Tosse", "baixa", "Gripe") // alta em outro dia: fora
	ontem := time.Now().AddDate(0, 0, -1)
	esquecida := model.Passagem{PacienteID: 2, Data: model.DataDe(ontem), Etapa: model.EtapaAguardandoTriagem, ChegadaEm: ontem}
	a.gravar(&esquecida) // aberta de ontem: dentro
	hoje := a.chegada(token, 1)
	a.atender(token, hoje.ID) // alta hoje: dentro ("Atendidos hoje")

	r := a.chamar(http.MethodGet, "/quadro", token, nil)
	exigir(t, r, http.StatusOK)
	quadro := ler[[]model.Passagem](t, r)

	ids := map[uint]model.Passagem{}
	for _, p := range quadro {
		ids[p.ID] = p
	}
	if _, ok := ids[antiga.ID]; ok || len(quadro) != 2 {
		t.Fatalf("quadro com %d passagens (%v); esperado a aberta de ontem e a alta de hoje", len(quadro), ids)
	}
	if p := ids[esquecida.ID]; p.Paciente == nil || p.Paciente.Nome != "Roberto" {
		t.Errorf("paciente não anexado: %+v", p.Paciente)
	}
	if p := ids[hoje.ID]; p.Anteriores != 1 {
		t.Errorf("passagens_anteriores = %d, esperado 1 (a vinda de 3 dias atrás)", p.Anteriores)
	}

	// Com o SOAP fora do ar o quadro continua funcionando, só sem os nomes.
	a.soap.derrubar(true)
	r = a.chamar(http.MethodGet, "/quadro", token, nil)
	exigir(t, r, http.StatusOK)
	for _, p := range ler[[]model.Passagem](t, r) {
		if p.Paciente != nil {
			t.Errorf("passagem %d com paciente %+v com o SOAP fora do ar", p.ID, p.Paciente)
		}
	}
}

func TestHistoricoDoPaciente(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dr.paulo")
	velha := a.passagemAntiga(1, 30, "Tosse", "baixa", "Gripe")
	recente := a.passagemAntiga(1, 2, "Dor", "media", "Tendinite")
	atual := a.chegada(token, 1)
	a.chegada(token, 2) // outro paciente não aparece

	r := a.chamar(http.MethodGet, "/pacientes/1/passagens", token, nil)
	exigir(t, r, http.StatusOK)
	ps := ler[[]model.Passagem](t, r)
	if len(ps) != 3 || ps[0].ID != atual.ID || ps[1].ID != recente.ID || ps[2].ID != velha.ID {
		t.Fatalf("histórico fora de ordem ou incompleto: %+v", ps)
	}
	if ps[1].Consulta == nil || ps[1].Consulta.Diagnostico != "Tendinite" {
		t.Errorf("documentos da vinda anterior não vieram: %+v", ps[1].Consulta)
	}
	exigir(t, a.chamar(http.MethodGet, "/pacientes/999/passagens", token, nil), http.StatusNotFound)
}

func TestResumoUsaSoAsVindasAnteriores(t *testing.T) {
	a := novoApp(t)
	token, _ := a.doutor("dra.quiteria")

	t.Run("primeira vinda", func(t *testing.T) {
		r := a.chamar(http.MethodGet, "/pacientes/2/resumo", token, nil)
		exigir(t, r, http.StatusOK)
		res := ler[dto.ResumoResponse](t, r)
		if res.TotalPassagens != 0 || res.GeradoPor != "regras" || !strings.Contains(res.Texto, "Primeira vinda") {
			t.Errorf("resumo = %+v", res)
		}
	})

	t.Run("com histórico", func(t *testing.T) {
		velha := a.passagemAntiga(1, 40, "Tosse", "baixa", "Gripe")
		recente := a.passagemAntiga(1, 5, "Dor no peito", "emergencia", "Angina")
		a.chegada(token, 1) // a passagem atual nunca entra

		r := a.chamar(http.MethodGet, "/pacientes/1/resumo", token, nil)
		exigir(t, r, http.StatusOK)
		res := ler[dto.ResumoResponse](t, r)
		if res.PacienteID != 1 || res.TotalPassagens != 2 || res.UltimaVinda != recente.Data {
			t.Fatalf("resumo = %+v", res)
		}
		if res.Passagens[0].PassagemID != recente.ID || res.Passagens[1].PassagemID != velha.ID {
			t.Errorf("passagens fora de ordem: %+v", res.Passagens)
		}
		for _, trecho := range []string{"2 passagens anteriores", "Diagnóstico: Angina", "Gripe", "emergência em 1 vinda"} {
			if !strings.Contains(res.Texto, trecho) {
				t.Errorf("texto sem %q: %s", trecho, res.Texto)
			}
		}
		if strings.Contains(res.Texto, "Dor de cabeça") {
			t.Errorf("a queixa da passagem atual entrou no resumo: %s", res.Texto)
		}
	})
}

// ---------------------------------------------------------------- finalização do plantão

func TestFinalizarPlantao(t *testing.T) {
	a := novoApp(t)
	token, doutorID := a.doutor("dr.rafael")
	outroToken, _ := a.doutor("dra.sara")

	pg := a.chegada(token, 1)
	exigir(t, a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/triagem"), token, triagem()), http.StatusOK)
	a.chegada(outroToken, 2) // movimentação de outro doutor

	r := a.chamar(http.MethodGet, "/finalizacao", token, nil)
	exigir(t, r, http.StatusOK)
	previa := ler[dto.PreviaFinalizacao](t, r)
	if len(previa.Movimentacoes) != 2 || previa.Pacientes != 1 ||
		previa.Totais[model.MovChegada] != 1 || previa.Totais[model.MovTriagem] != 1 {
		t.Fatalf("prévia = %+v", previa)
	}
	triagemMov := previa.Movimentacoes[1]
	if triagemMov.Tipo != model.MovTriagem || triagemMov.PacienteNome != "José" ||
		!strings.Contains(triagemMov.Descricao, "prioridade alta") {
		t.Errorf("movimentação da triagem = %+v", triagemMov)
	}

	// Comentário longo demais é recusado e nada é finalizado.
	exigir(t, a.chamar(http.MethodPost, "/finalizacao", token, map[string]any{"comentario": strings.Repeat("x", 2001)}), http.StatusBadRequest)

	outraMov := ler[dto.PreviaFinalizacao](t, a.chamar(http.MethodGet, "/finalizacao", outroToken, nil)).Movimentacoes[0]

	esperarProximoSegundo()
	r = a.chamar(http.MethodPost, "/finalizacao", token, map[string]any{
		"comentario": "  Plantão tranquilo  ",
		"comentarios": []map[string]any{
			{"movimentacao_id": triagemMov.ID, "comentario": "Reavaliar se a febre voltar"},
			{"movimentacao_id": outraMov.ID, "comentario": "não é minha"}, // ignorado
		},
	})
	exigir(t, r, http.StatusCreated)
	fin := ler[model.Finalizacao](t, r)
	if fin.DoutorID != doutorID || fin.Comentario != "Plantão tranquilo" || len(fin.Movimentacoes) != 2 {
		t.Fatalf("finalização = %+v", fin)
	}
	if fin.Movimentacoes[1].Comentario != "Reavaliar se a febre voltar" {
		t.Errorf("comentário da triagem não gravado: %+v", fin.Movimentacoes[1])
	}

	// O token antigo deixa de valer: o doutor foi deslogado no servidor.
	r = a.chamar(http.MethodGet, "/auth/me", token, nil)
	exigir(t, r, http.StatusUnauthorized)
	if e := erroDe(t, r); !strings.Contains(e, "sessão encerrada") {
		t.Errorf("error = %q", e)
	}

	// Novo login funciona e começa um plantão vazio.
	novo := a.login("dr.rafael", "senha123")
	previa = ler[dto.PreviaFinalizacao](t, a.chamar(http.MethodGet, "/finalizacao", novo, nil))
	if len(previa.Movimentacoes) != 0 {
		t.Errorf("o novo plantão já começou com %d movimentação(ões)", len(previa.Movimentacoes))
	}
	r = a.chamar(http.MethodGet, "/finalizacoes", novo, nil)
	exigir(t, r, http.StatusOK)
	if fs := ler[[]model.Finalizacao](t, r); len(fs) != 1 || fs[0].ID != fin.ID {
		t.Errorf("finalizações = %+v", fs)
	}

	// O comentário aparece para todos na passagem do paciente.
	r = a.chamar(http.MethodGet, rotaPassagem(pg.ID, ""), outroToken, nil)
	exigir(t, r, http.StatusOK)
	if cs := ler[model.Passagem](t, r).Comentarios; len(cs) != 1 || cs[0].Comentario != "Reavaliar se a febre voltar" {
		t.Errorf("comentários da passagem = %+v", cs)
	}

	// O outro doutor continua logado, com a movimentação dele pendente e sem comentário.
	previa = ler[dto.PreviaFinalizacao](t, a.chamar(http.MethodGet, "/finalizacao", outroToken, nil))
	if len(previa.Movimentacoes) != 1 || previa.Movimentacoes[0].Comentario != "" {
		t.Errorf("prévia do outro doutor = %+v", previa.Movimentacoes)
	}
}

// ---------------------------------------------------------------- quadro em tempo real

func TestWebSocketExigeToken(t *testing.T) {
	a := novoApp(t)
	r := a.chamar(http.MethodGet, "/ws", "", nil)
	exigir(t, r, http.StatusUnauthorized)
	r = a.chamar(http.MethodGet, "/ws?token=invalido", "", nil)
	exigir(t, r, http.StatusUnauthorized)
}

func TestQuadroEmTempoReal(t *testing.T) {
	a := novoApp(t)
	tokenA, idA := a.doutor("dra.tania")
	tokenB, _ := a.doutor("dr.ulisses")
	pg := a.chegada(tokenA, 1)

	// init traz o doutor, as passagens do quadro e a geometria.
	wsA := a.conectar(tokenA)
	init := wsA.esperar("init")
	eu := init["you"].(map[string]any)
	if uint(eu["doutor_id"].(float64)) != idA || eu["nome"] != "dra.tania" {
		t.Errorf("you = %v", eu)
	}
	if ps := init["passagens"].([]any); len(ps) != 1 {
		t.Errorf("init com %d passagens, esperado 1", len(ps))
	}
	if q := init["quadro"].(map[string]any); q["largura"].(float64) != model.QuadroLargura {
		t.Errorf("geometria do quadro = %v", q)
	}

	// Presença: A vê B entrar e o cursor dele.
	wsB := a.conectar(tokenB)
	if outros := wsB.esperar("init")["doutores"].([]any); len(outros) != 1 {
		t.Errorf("B vê %d doutores conectados, esperado 1", len(outros))
	}
	join := wsA.esperar("join")["doutor"].(map[string]any)
	connB := join["conn_id"]
	wsB.enviar(map[string]any{"type": "cursor", "x": 100, "y": 200})
	if c := wsA.esperar("cursor"); c["conn_id"] != connB || c["x"].(float64) != 100 {
		t.Errorf("cursor = %v", c)
	}

	// Lock do arraste: com B segurando o card, A não consegue pegá-lo.
	wsB.enviar(map[string]any{"type": "drag_start", "passagem_id": pg.ID})
	if m := wsA.esperar("drag_start"); m["conn_id"] != connB {
		t.Errorf("drag_start = %v", m)
	}
	wsA.enviar(map[string]any{"type": "drag_start", "passagem_id": pg.ID})
	if m := wsA.esperar("drag_denied"); m["conn_id"] != connB {
		t.Errorf("drag_denied = %v", m)
	}
	wsB.enviar(map[string]any{"type": "drag_move", "passagem_id": pg.ID, "x": 250, "y": 300})
	if m := wsA.esperar("drag_move"); m["x"].(float64) != 250 {
		t.Errorf("drag_move = %v", m)
	}

	// Soltar na mesma coluna grava a posição e avisa todos.
	wsB.enviar(map[string]any{"type": "drag_end", "passagem_id": pg.ID, "x": 300, "y": 400})
	if p := wsA.esperarPassagem(pg.ID); p["pos_x"].(float64) != 300 || p["pos_y"].(float64) != 400 {
		t.Errorf("posição avisada = (%v, %v)", p["pos_x"], p["pos_y"])
	}
	gravada := ler[model.Passagem](t, a.chamar(http.MethodGet, rotaPassagem(pg.ID, ""), tokenA, nil))
	if gravada.PosX != 300 || gravada.PosY != 400 {
		t.Errorf("posição gravada = (%v, %v)", gravada.PosX, gravada.PosY)
	}

	// Soltar em outra coluna não muda a etapa: o card volta.
	wsA.enviar(map[string]any{"type": "drag_start", "passagem_id": pg.ID})
	wsB.esperar("drag_start")
	wsA.enviar(map[string]any{"type": "drag_end", "passagem_id": pg.ID, "x": 1200, "y": 400})
	if p := wsB.esperarPassagem(pg.ID); p["etapa"] != model.EtapaAguardandoTriagem || p["pos_x"].(float64) != 300 {
		t.Errorf("card solto em outra coluna: etapa %v, pos_x %v", p["etapa"], p["pos_x"])
	}

	// Mudanças pela API REST chegam no quadro.
	exigir(t, a.chamar(http.MethodPost, rotaPassagem(pg.ID, "/triagem"), tokenA, triagem()), http.StatusOK)
	if p := wsB.esperarPassagem(pg.ID); p["etapa"] != model.EtapaAguardandoConsulta {
		t.Errorf("etapa avisada = %v", p["etapa"])
	}
	exigir(t, a.chamar(http.MethodPut, "/pacientes/1", tokenA, map[string]string{"nome": "José Silva", "status": "estável"}), http.StatusOK)
	if p := wsB.esperar("paciente_updated")["paciente"].(map[string]any); p["nome"] != "José Silva" {
		t.Errorf("paciente avisado = %v", p)
	}

	// B sai: A recebe leave.
	wsB.conn.Close()
	if m := wsA.esperar("leave"); m["conn_id"] != connB {
		t.Errorf("leave = %v", m)
	}
}

func TestCardSeguradoEhLiberadoQuandoQuemArrastaSai(t *testing.T) {
	a := novoApp(t)
	tokenA, _ := a.doutor("dra.vera")
	tokenB, _ := a.doutor("dr.wagner")
	pg := a.chegada(tokenA, 1)

	wsA, wsB := a.conectar(tokenA), a.conectar(tokenB)
	wsA.esperar("init")
	wsB.esperar("init")
	wsA.enviar(map[string]any{"type": "drag_start", "passagem_id": pg.ID})
	wsB.esperar("drag_start")

	wsA.conn.Close()
	wsB.esperarPassagem(pg.ID) // o card volta para a posição gravada
	wsB.esperar("leave")

	wsB.enviar(map[string]any{"type": "drag_start", "passagem_id": pg.ID})
	wsB.enviar(map[string]any{"type": "drag_end", "passagem_id": pg.ID, "x": 200, "y": 150})
	if p := wsB.esperarPassagem(pg.ID); p["pos_x"].(float64) != 200 {
		t.Errorf("B não conseguiu mover o card liberado: pos_x = %v", p["pos_x"])
	}
}

func TestFinalizarDesconectaOQuadroDoDoutor(t *testing.T) {
	a := novoApp(t)
	tokenA, _ := a.doutor("dr.xavier")
	tokenB, _ := a.doutor("dra.yara")

	abaA1, abaA2, wsB := a.conectar(tokenA), a.conectar(tokenA), a.conectar(tokenB)
	for _, ws := range []*clienteWS{abaA1, abaA2, wsB} {
		ws.esperar("init")
	}

	exigir(t, a.chamar(http.MethodPost, "/finalizacao", tokenA, map[string]any{}), http.StatusCreated)

	// As duas abas de A recebem o aviso e são fechadas pelo servidor.
	for _, aba := range []*clienteWS{abaA1, abaA2} {
		aba.esperar("sessao_encerrada")
		aba.esperarFechamento()
	}
	// B continua conectado e vê A sair.
	wsB.esperar("leave")
	wsB.esperar("leave")
	wsB.enviar(map[string]any{"type": "cursor", "x": 1, "y": 1})
	exigir(t, a.chamar(http.MethodGet, "/auth/me", tokenB, nil), http.StatusOK)
}
