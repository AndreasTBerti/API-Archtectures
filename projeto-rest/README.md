# projeto-rest — Controle de Pacientes

API REST (Gin + GORM/SQLite) para controle de pacientes. Os **doutores** fazem login. Os
dados do **paciente** ficam no serviço SOAP (`../projeto-soap`), que tem o próprio banco.
Este REST guarda os documentos gerados em cada vinda do paciente e as finalizações de
plantão. Não existe sincronização: o REST consulta o SOAP sempre que precisa.

## Rodando

```bash
# 1. suba o SOAP (em outro terminal)
cd ../projeto-soap && uv run python server.py     # http://127.0.0.1:8000/

# 2. suba o REST
go run .            # ou: air
# Swagger: http://localhost:8080/swagger
```

Variáveis de ambiente (todas opcionais):

| Variável          | Padrão                    | Descrição                          |
|-------------------|---------------------------|------------------------------------|
| `JWT_SECRET`      | segredo de desenvolvimento| Chave HS256 usada para assinar JWT |
| `JWT_TTL_MINUTES` | `60`                      | Validade do token                  |
| `SOAP_URL`        | `http://127.0.0.1:8000/`  | Endereço do `ProntuarioService`    |
| `PORT`            | `8080`                    | Porta HTTP                         |
| `REST_DB`         | `rest.db`                 | Caminho do banco SQLite            |

## Testes

```bash
go test ./...                    # integração: rápido, não precisa do SOAP
go test -tags e2e ./e2e          # ponta a ponta: sobe o SOAP real e o binário do REST
```

**Integração** (`integracao_test.go`): cada teste sobe a aplicação inteira (router,
middleware JWT, handlers, repositórios e cliente SOAP) num servidor HTTP de teste, com um
SQLite temporário. O SOAP é substituído por um servidor falso (`soapfalso_test.go`) que
responde com o mesmo XML do Spyne, inclusive os Faults de validação, e que pode ser
"derrubado" para testar o 502. Cobre autenticação, doutores, pacientes, passagens, resumo,
quadro, finalização do plantão e o WebSocket (presença, cursor, lock de arraste, avisos).

**Ponta a ponta** (`e2e/`): compila o REST, sobe o `../projeto-soap/server.py` com o Python do
`.venv` (cria com `uv sync` se faltar) e roda as jornadas pela rede, com dois doutores
conectados ao quadro. Cada servidor usa uma porta livre e um banco temporário: `rest.db` e
`prontuarios.db` não são tocados. O último teste derruba o SOAP para conferir que o quadro
continua de pé.

## Autenticação (OAuth2 *password grant* + JWT Bearer)

```bash
# registrar um doutor
curl -X POST localhost:8080/auth/register -H 'Content-Type: application/json' \
     -d '{"nome":"dra.ana","senha":"123456"}'

# obter token — JSON...
curl -X POST localhost:8080/auth/token -H 'Content-Type: application/json' \
     -d '{"nome":"dra.ana","senha":"123456"}'
# ...ou no formato form-urlencoded do OAuth2
curl -X POST localhost:8080/auth/token \
     -d 'grant_type=password&username=dra.ana&password=123456'
# => {"access_token":"<jwt>","token_type":"Bearer","expires_in":3600}

# usar o token
curl localhost:8080/auth/me -H 'Authorization: Bearer <jwt>'
```

Só `/auth/register`, `/auth/token`, `/health`, `/ping` e `/swagger` são públicos.
Todo o resto exige `Authorization: Bearer <jwt>` (no Swagger, botão **Authorize**).

## Pacientes (serviço SOAP)

Todas as rotas abaixo são repassadas ao `ProntuarioService`. O `id` do paciente é o
`id_paciente` do SOAP, e é por ele que as passagens do REST se ligam ao paciente.

| Método | Rota              | Operação SOAP        | Descrição                                   |
|--------|-------------------|----------------------|---------------------------------------------|
| GET    | `/pacientes`      | `list_pacientes`     | Lista em ordem alfabética                   |
| POST   | `/pacientes`      | `criar_paciente`     | Cadastra (`{"nome", "status"}`)             |
| GET    | `/pacientes/{id}` | `get_paciente`       | Busca                                       |
| PUT    | `/pacientes/{id}` | `atualizar_paciente` | Atualiza nome e status                      |
| DELETE | `/pacientes/{id}` | `remover_paciente`   | Remove no SOAP e apaga as passagens no REST |

Erros de validação do SOAP viram 400, paciente inexistente vira 404 e SOAP fora do ar vira
502. Não é possível remover um paciente que está no hospital. Cadastrar não coloca ninguém
no quadro: o paciente entra quando a chegada dele é registrada.

Se o SOAP cair, o quadro continua funcionando com os documentos do REST, mas os cards mostram
só o número do prontuário no lugar do nome.

## Passagens: cada vinda ao hospital

Uma **passagem** é uma vinda do paciente ao hospital, em um dia. Existe no máximo uma por
paciente por dia, e cada passagem tem os seus próprios documentos (admissão, triagem e
consulta). Por isso um paciente que volta começa com formulários vazios, e os dados das
vindas anteriores aparecem só no resumo e no histórico.

| Etapa                 | Como chega nela                                                       |
|-----------------------|-----------------------------------------------------------------------|
| `aguardando_triagem`  | `POST /pacientes/{id}/passagens` com a admissão (registrar chegada)   |
| `aguardando_consulta` | `POST /passagens/{id}/triagem`                                        |
| `alta`                | `POST /passagens/{id}/consulta` (exige triagem). O card sai do quadro |

Regras:

- Se o paciente já está no hospital, registrar chegada responde 409.
- Se ele teve alta hoje e voltou, a passagem do dia é reaberta em `aguardando_triagem`.
- Se ficou uma passagem aberta de outro dia, é preciso registrar a consulta dela antes.
- Passagens encerradas não podem ser editadas. Numa passagem aberta,
  `POST /passagens/{id}/admissao` e um novo envio da triagem corrigem os documentos.
- Quem registrou cada documento fica em `autor_id`, só para auditoria.

| Método | Rota                         | Descrição                                               |
|--------|------------------------------|---------------------------------------------------------|
| GET    | `/quadro`                    | Passagens abertas e as de hoje que já tiveram alta      |
| GET    | `/pacientes/{id}/passagens`  | Histórico do paciente, da vinda mais recente à mais antiga |
| POST   | `/pacientes/{id}/passagens`  | Registra a chegada (abre ou reabre a passagem de hoje)  |
| GET    | `/pacientes/{id}/resumo`     | Resumo baseado só nas vindas anteriores                 |
| GET    | `/passagens/{id}`            | Uma passagem com paciente e documentos                  |
| POST   | `/passagens/{id}/admissao`   | Corrige a admissão                                      |
| POST   | `/passagens/{id}/triagem`    | Registra a triagem                                      |
| POST   | `/passagens/{id}/consulta`   | Registra a consulta e dá alta                           |

### Resumo do paciente

O resumo usa apenas passagens **encerradas de dias anteriores**. A passagem atual nunca
entra, e o filtro é feito no backend. A resposta traz os dados estruturados de cada vinda
anterior e um campo `texto`. Hoje esse texto é montado por regras em
`internals/resumo/resumo.go`. Para usar IA, basta criar outra implementação da interface
`resumo.Gerador` e passá-la em `main.go`: o contrato da rota não muda.

### Migração de bancos antigos

Na inicialização, bancos de versões anteriores são convertidos:

- Documentos no formato antigo, um de cada tipo por paciente, viram uma passagem por
  paciente, datada pelo primeiro documento.
- A antiga tabela local de pacientes é removida. O `paciente_id` das passagens passa a ser o
  `soap_id` que ela guardava.

## Finalizar plantão

O botão **Finalizar** mostra tudo o que o doutor registrou desde a última finalização:
chegadas, triagens, consultas com alta e correções, cada uma com hora, paciente e um resumo
do que foi registrado. O doutor pode comentar qualquer movimentação e deixar um comentário
geral. Ao confirmar, a finalização é gravada e o doutor é deslogado.

O logout acontece no servidor, não só na tela. Tokens emitidos antes da finalização passam a
receber 401, e as conexões do quadro desse doutor são fechadas, inclusive em outras abas. Os
comentários ficam visíveis para todos na passagem do paciente.

| Método | Rota            | Descrição                                                    |
|--------|-----------------|--------------------------------------------------------------|
| GET    | `/finalizacao`  | Resumo das movimentações pendentes do doutor autenticado     |
| POST   | `/finalizacao`  | Finaliza com os comentários e encerra a sessão               |
| GET    | `/finalizacoes` | Plantões já finalizados pelo doutor                          |

Corpo do POST, com comentários opcionais:

```json
{"comentario": "Plantão tranquilo",
 "comentarios": [{"movimentacao_id": 3, "comentario": "Reavaliar se a febre voltar"}]}
```

## Quadro em tempo real (WebSocket + canvas)

Abra `http://localhost:8080/` (arquivo único `web/index.html`). Cada doutor faz login e vê as
passagens abertas nas colunas **Aguardando triagem** e **Aguardando consulta**, o **cursor
dos outros doutores** e os cards sendo arrastados por eles em tempo real. Render em
`requestAnimationFrame` (60 fps) com interpolação das posições remotas; cada cliente envia
no máximo 1 mensagem de cursor e 1 de arraste por frame.

O card mostra o nome, a hora de chegada e o tempo de espera, a queixa principal, a
prioridade da triagem e um selo "2ª vinda" quando o paciente já esteve no hospital.

- Clique simples no card: abre o painel lateral do paciente, só para leitura.
- O painel tem três abas: **Passagem atual** (etapas, documentos e o próximo passo),
  **Resumo** (vindas anteriores) e **Histórico** (todas as passagens por data). O começo do
  resumo também aparece no topo da passagem atual.
- Os formulários abrem dentro do painel, quando o doutor escolhe o próximo passo.
- Arrastar para a próxima coluna abre a triagem no painel; o card fica "segurado" (os
  outros veem) até salvar ou cancelar.
- Arrastar dentro da coluna reposiciona o card. Voltar etapa é bloqueado.
- **Pacientes** (barra superior): busca, cadastro de paciente novo e botão para registrar a
  chegada. A lista vem direto do SOAP.
- **Finalizar** (barra superior): resumo do plantão, comentários e logout.
- **Atendidos hoje** (barra superior): quem teve alta hoje.
- Enquanto um doutor arrasta um card, os outros não conseguem pegá-lo (lock no servidor).

Endpoint: `GET /ws?token=<jwt>`. Protocolo documentado em `internals/realtime/hub.go`.

## Estrutura

```
internals/
  auth/        geração e validação de JWT
  middleware/  middleware Bearer JWT
  soap/        cliente SOAP (envelope manual + encoding/xml): get, list, criar, atualizar, remover
  realtime/    hub WebSocket do quadro (presença, cursores, arraste)
  pacientes/   acesso aos pacientes do serviço SOAP
  resumo/      resumo do paciente a partir das passagens anteriores
  dto/ model/ repository/ handler/
web/index.html frontend do quadro (html+css+js em um arquivo)
```

Após alterar anotações Swagger: `swag init`.
