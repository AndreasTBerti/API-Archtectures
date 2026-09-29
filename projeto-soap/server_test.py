import pytest
from spyne.model.fault import Fault
from webtest import TestApp

import banco
from server import ProntuarioService, ProntuarioModel, wsgi_app


@pytest.fixture(autouse=True)
def banco_temporario(tmp_path, monkeypatch):
    # Cada teste usa um banco novo, já com os pacientes iniciais.
    monkeypatch.setenv("SOAP_DB", str(tmp_path / "prontuarios_teste.db"))


@pytest.fixture
def service():
    return ProntuarioService()


@pytest.fixture
def client():
    # Envelopa a aplicação WSGI do Spyne para simular requisições HTTP
    return TestApp(wsgi_app)


def soap(client, operacao, corpo=""):
    xml_payload = f"""<?xml version="1.0" encoding="UTF-8"?>
    <soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cli="clinica.soap">
       <soapenv:Header/>
       <soapenv:Body><cli:{operacao}>{corpo}</cli:{operacao}></soapenv:Body>
    </soapenv:Envelope>"""
    headers = {'Content-Type': 'text/xml; charset=utf-8', 'SOAPAction': f'"{operacao}"'}
    return client.post('/', params=xml_payload.encode("utf-8"), headers=headers, expect_errors=True)


# ---- chamadas diretas ao serviço ----

def test_get_paciente_existente(service):
    resultado = service.get_paciente(None, 1)

    assert isinstance(resultado, ProntuarioModel)
    assert resultado.id_paciente == 1
    assert resultado.paciente == "José"
    assert resultado.status == "doente"
    assert resultado.criado_em
    assert resultado.mensagem_erro is None


def test_get_paciente_inexistente(service):
    resultado = service.get_paciente(None, 99)

    assert resultado.mensagem_erro == "Paciente não encontrado"
    assert resultado.paciente is None


def test_list_pacientes_em_ordem_alfabetica(service):
    resultado = service.list_pacientes(None)

    assert [p.paciente for p in resultado] == ["José", "Roberto"]


def test_criar_paciente_persiste(service):
    novo = service.criar_paciente(None, "  Ana Maria  ", "em observação")

    assert novo.id_paciente == 3
    assert novo.paciente == "Ana Maria"
    # Outra instância do serviço lê do mesmo banco: o dado ficou gravado.
    assert ProntuarioService().get_paciente(None, 3).paciente == "Ana Maria"
    assert banco.buscar(3)["status"] == "em observação"


def test_criar_paciente_sem_nome_gera_fault(service):
    with pytest.raises(Fault) as erro:
        service.criar_paciente(None, "   ", "doente")
    assert erro.value.faultcode == "Client.Validacao"


def test_atualizar_paciente(service):
    atualizado = service.atualizar_paciente(None, 2, "Roberto Silva", "doente")

    assert atualizado.paciente == "Roberto Silva"
    assert service.get_paciente(None, 2).status == "doente"


def test_atualizar_paciente_inexistente(service):
    assert service.atualizar_paciente(None, 99, "X", "").mensagem_erro == "Paciente não encontrado"


def test_remover_paciente(service):
    removido = service.remover_paciente(None, 1)

    assert removido.paciente == "José"
    assert service.get_paciente(None, 1).mensagem_erro == "Paciente não encontrado"
    assert [p.id_paciente for p in service.list_pacientes(None)] == [2]


def test_remover_paciente_inexistente(service):
    assert service.remover_paciente(None, 99).mensagem_erro == "Paciente não encontrado"


# ---- pelo protocolo SOAP (XML) ----

def test_wsdl_disponivel(client):
    response = client.get('/?wsdl')
    assert response.status_code == 200
    assert 'wsdl:definitions' in response.text
    for operacao in ("get_paciente", "list_pacientes", "criar_paciente", "atualizar_paciente", "remover_paciente"):
        assert operacao in response.text


def test_soap_request_xml(client):
    response = soap(client, "get_paciente", "<cli:id_paciente>1</cli:id_paciente>")

    assert response.status_code == 200
    # O modelo está no namespace clinica.soap, então o Spyne usa o prefixo tns:
    assert '<tns:paciente>José</tns:paciente>' in response.text
    assert '<tns:status>doente</tns:status>' in response.text


def test_soap_list_pacientes_xml(client):
    response = soap(client, "list_pacientes")

    assert response.status_code == 200
    assert response.text.count('<tns:ProntuarioModel>') == 2
    assert '<tns:paciente>Roberto</tns:paciente>' in response.text


def test_soap_criar_paciente_xml(client):
    response = soap(client, "criar_paciente", "<cli:paciente>Carla</cli:paciente><cli:status>doente</cli:status>")

    assert response.status_code == 200
    assert '<tns:id_paciente>3</tns:id_paciente>' in response.text
    assert soap(client, "list_pacientes").text.count('<tns:ProntuarioModel>') == 3


def test_soap_criar_paciente_invalido_xml(client):
    response = soap(client, "criar_paciente", "<cli:paciente></cli:paciente>")

    assert response.status_code == 500
    assert 'Client.Validacao' in response.text
    assert 'O nome do paciente é obrigatório' in response.text
