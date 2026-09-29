import os
import subprocess
import sys
import time

import pytest
from streamlit.testing.v1 import AppTest


# 1. Configura o servidor SOAP para rodar durante o teste, com um banco temporário
@pytest.fixture(scope="module", autouse=True)
def soap_server(tmp_path_factory):
    print("\nIniciando servidor SOAP para teste E2E...")
    # Porta própria para não colidir com um servidor de desenvolvimento na 8000
    os.environ["SOAP_WSDL"] = "http://127.0.0.1:8010/?wsdl"
    env = dict(os.environ, SOAP_PORT="8010", SOAP_DB=str(tmp_path_factory.mktemp("soap") / "prontuarios_e2e.db"))
    process = subprocess.Popen([sys.executable, "server.py"], env=env)

    # Aguarda 2 segundos para garantir que a porta está pronta
    time.sleep(2)

    yield  # Executa os testes do arquivo

    process.terminate()
    print("\nServidor SOAP encerrado.")


# 2. Testa o fluxo completo pela interface
def test_fluxo_sucesso_prontuario():
    at = AppTest.from_file("app.py", default_timeout=30).run()

    assert "Consulta de Prontuário Médico" in at.title[0].value

    at.number_input[0].set_value(1).run()
    at.button[1].click().run()  # button[0] é "Reconectar ao servidor SOAP"

    # O Streamlit chamou o Zeep, que chamou o Spyne, que leu o SQLite
    assert at.success[0].value == "Prontuário encontrado com sucesso!"
    assert at.metric[0].label == "Paciente"
    assert at.metric[0].value == "José"
    assert at.metric[1].label == "Status"
    assert at.metric[1].value == "doente"


# 3. Testa o fluxo de erro pela interface
def test_fluxo_paciente_nao_encontrado():
    at = AppTest.from_file("app.py", default_timeout=30).run()

    at.number_input[0].set_value(99).run()
    at.button[1].click().run()

    assert "Paciente não encontrado" in at.warning[0].value


# 4. Cadastra um paciente pela interface e confere que ele foi gravado
def test_fluxo_cadastro_paciente():
    at = AppTest.from_file("app.py", default_timeout=30).run()

    at.text_input[0].input("Ana Maria")
    at.text_input[1].input("em observação")
    at.button[2].click().run()  # "Cadastrar" do formulário

    assert "Ana Maria" in at.success[0].value
    nomes = list(at.dataframe[0].value["Paciente"])
    assert "Ana Maria" in nomes
