import pytest
import subprocess
import time
from streamlit.testing.v1 import AppTest

# 1. Configura o servidor SOAP para rodar durante o teste
@pytest.fixture(scope="module", autouse=True)
def soap_server():
    # Inicia o backend em segundo plano
    print("\nIniciando servidor SOAP para teste E2E...")
    process = subprocess.Popen(["python", "server.py"])
    
    # Aguarda 2 segundos para garantir que a porta 8000 está pronta
    time.sleep(2) 
    
    yield # Executa os testes do arquivo
    
    # Encerra o servidor após os testes terminarem
    process.terminate()
    print("\nServidor SOAP encerrado.")

# 2. Testa o fluxo completo pela interface
def test_fluxo_sucesso_prontuario():
    # Inicializa o Streamlit simulado carregando o seu app.py
    at = AppTest.from_file("app.py", default_timeout=10).run()
    
    # Verifica se o título carregou
    assert "Consulta de Prontuário Médico" in at.title[0].value

    # Simula a digitação do ID '1' no campo number_input
    at.number_input[0].set_value(1).run()
    
    # Simula o clique no botão "Solicitar Prontuário"
    at.button[0].click().run()
    
    # Validações: O Streamlit deve ter chamado o Zeep, que chamou o Spyne e retornou a tela
    assert at.success[0].value == "Prontuário encontrado com sucesso!"
    
    # Valida se os dados do paciente "José" apareceram nos cards (st.metric)
    assert at.metric[0].label == "Paciente"
    assert at.metric[0].value == "José"
    
    assert at.metric[1].label == "Status"
    assert at.metric[1].value == "doente"

# 3. Testa o fluxo de erro pela interface
def test_fluxo_paciente_nao_encontrado():
    at = AppTest.from_file("app.py", default_timeout=10).run()
    
    # Simula buscar o ID '99' (inexistente)
    at.number_input[0].set_value(99).run()
    at.button[0].click().run()
    
    # Valida se o st.warning de erro apareceu com a mensagem do SOAP
    assert "Paciente não encontrado" in at.warning[0].value