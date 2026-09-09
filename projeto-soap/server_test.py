import pytest
from webtest import TestApp

# Importa o serviço e o modelo do seu arquivo do servidor
from server import ProntuarioService, ProntuarioModel, wsgi_app

def test_get_paciente_existente():
    # Instancia a classe do serviço
    service = ProntuarioService()
    
    # Chama o método passando ctx=None e o ID do paciente
    resultado = service.get_paciente(None, 1)
    
    # Asserções de teste
    assert isinstance(resultado, ProntuarioModel)
    assert resultado.paciente == "José"
    assert resultado.status == "doente"
    assert resultado.mensagem_erro is None

def test_get_paciente_inexistente():
    service = ProntuarioService()
    
    resultado = service.get_paciente(None, 99)
    
    assert isinstance(resultado, ProntuarioModel)
    assert resultado.mensagem_erro == "Paciente não encontrado"
    assert resultado.paciente is None

@pytest.fixture
def client():
    # Envelopa a aplicação WSGI do Spyne para simular requisições HTTP
    return TestApp(wsgi_app)

def test_wsdl_disponivel(client):
    # Testa se o contrato WSDL está respondendo corretamente
    response = client.get('/?wsdl')
    assert response.status_code == 200
    assert 'wsdl:definitions' in response.text

def test_soap_request_xml(client):
    xml_payload = """<?xml version="1.0" encoding="UTF-8"?>
    <soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/" xmlns:cli="clinica.soap">
       <soapenv:Header/>
       <soapenv:Body>
          <cli:get_paciente>
             <cli:id_paciente>1</cli:id_paciente>
          </cli:get_paciente>
       </soapenv:Body>
    </soapenv:Envelope>"""

    headers = {
        'Content-Type': 'text/xml; charset=utf-8',
        'SOAPAction': '"get_paciente"'
    }

    response = client.post('/', params=xml_payload, headers=headers)

    assert response.status_code == 200
    # Adicionado o prefixo s0: gerado pelo Spyne para o modelo
    assert '<s0:paciente>José</s0:paciente>' in response.text
    assert '<s0:status>doente</s0:status>' in response.text