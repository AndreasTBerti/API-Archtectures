# projeto-soap — ProntuarioService

Serviço SOAP (Spyne) dono dos dados do paciente. Tudo o que lida diretamente com o paciente
passa por aqui: cadastro, consulta, listagem, alteração e remoção. O `../projeto-rest` não
guarda pacientes: ele chama este serviço e guarda só os documentos gerados em cada vinda.

```bash
uv run python server.py          # http://127.0.0.1:8000/  (WSDL em /?wsdl)
uv run streamlit run app.py      # painel: consulta, cadastro e lista de pacientes
uv run --with pytest --with webtest pytest
```

## Persistência

Os pacientes ficam em SQLite, no arquivo `prontuarios.db` ao lado do `server.py`
(`banco.py`). Na primeira execução, com o banco vazio, são criados José (1) e Roberto (2).

| Variável     | Padrão                          | Descrição                              |
|--------------|---------------------------------|----------------------------------------|
| `SOAP_DB`    | `prontuarios.db`                | Caminho do banco                       |
| `SOAP_PORT`  | `8000`                          | Porta do servidor                      |
| `SOAP_WSDL`  | `http://127.0.0.1:8000/?wsdl`   | WSDL usado pelo painel Streamlit       |

Os testes usam um banco temporário e não mexem no `prontuarios.db`. O teste de ponta a ponta
sobe o servidor na porta 8010.

## Contrato (`contrato_servico.wsdl`)

`ProntuarioModel`:

| Campo           | Tipo    | Descrição                                           |
|-----------------|---------|-----------------------------------------------------|
| `id_paciente`   | integer | ID do paciente (é o número do prontuário)           |
| `paciente`      | string  | Nome                                                |
| `status`        | string  | Situação clínica                                    |
| `criado_em`     | string  | Data de cadastro (ISO 8601, UTC)                    |
| `atualizado_em` | string  | Última alteração (ISO 8601, UTC)                    |
| `mensagem_erro` | string  | Preenchido apenas quando o paciente não existe      |

Operações:

- `get_paciente(id_paciente)` → `ProntuarioModel`
- `list_pacientes()` → `ProntuarioModel[]`, em ordem alfabética
- `criar_paciente(paciente, status)` → `ProntuarioModel` criado
- `atualizar_paciente(id_paciente, paciente, status)` → `ProntuarioModel` atualizado
- `remover_paciente(id_paciente)` → `ProntuarioModel` removido

Paciente inexistente volta com `mensagem_erro`. Nome vazio ou longo demais gera um SOAP Fault
com `faultcode` `Client.Validacao`.

O campo `id_doutor` saiu do contrato: o paciente não tem mais um doutor responsável.

Para regenerar o WSDL após alterar o serviço:

```bash
PYTHONPATH=. uv run python -c "from spyne.interface.wsdl import Wsdl11; from server import application; w=Wsdl11(application.interface); w.build_interface_document('http://127.0.0.1:8000/'); open('contrato_servico.wsdl','wb').write(w.get_interface_document())"
```
