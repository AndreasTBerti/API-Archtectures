import os

import streamlit as st
from zeep import Client
from zeep.exceptions import Fault

st.set_page_config(page_title="Painel Prontuário SOAP", layout="centered")
st.title("Consulta de Prontuário Médico 🏥")

WSDL_URL = os.environ.get('SOAP_WSDL', 'http://127.0.0.1:8000/?wsdl')

@st.cache_resource
def get_soap_client():
    try:
        return Client(wsdl=WSDL_URL)
    except Exception as e:
        st.error(f"Erro ao conectar com o servidor SOAP: {e}")
        return None

client = get_soap_client()

# O cliente Zeep fica em cache com o WSDL lido na primeira conexão. Se o
# server.py for reiniciado com um contrato novo, é preciso reconectar.
if st.button("Reconectar ao servidor SOAP"):
    get_soap_client.clear()
    st.rerun()

if client:
    st.header("Serviço de Prontuário")

    # Campo configurado com step=1 para garantir entrada de inteiros
    id_paciente = st.number_input("Digite o ID do Paciente:", min_value=1, step=1, value=1)

    if st.button("Solicitar Prontuário"):
        # Executa a consulta no servidor SOAP
        resposta = client.service.get_paciente(id_paciente=int(id_paciente))

        # Tratamento do objeto ProntuarioModel
        if getattr(resposta, "mensagem_erro", None):
            st.warning(f"⚠️ {resposta.mensagem_erro}")
        else:
            st.success("Prontuário encontrado com sucesso!")

            col1, col2 = st.columns(2)
            with col1:
                st.metric(label="Paciente", value=resposta.paciente)
            with col2:
                st.metric(label="Status", value=resposta.status or "—")

    st.header("Novo paciente")
    with st.form("novo_paciente", clear_on_submit=True):
        nome = st.text_input("Nome")
        status = st.text_input("Status", placeholder="ex.: em observação")
        if st.form_submit_button("Cadastrar"):
            try:
                novo = client.service.criar_paciente(paciente=nome, status=status)
                st.success(f"Paciente {novo.paciente} cadastrado com o ID {novo.id_paciente}")
            except Fault as e:
                st.warning(f"⚠️ {e.message}")

    st.header("Pacientes cadastrados")
    pacientes = client.service.list_pacientes() or []
    st.dataframe(
        [{"ID": p.id_paciente, "Paciente": p.paciente, "Status": p.status} for p in pacientes],
        hide_index=True, use_container_width=True,
    )
else:
    st.error("Servidor SOAP indisponível. Verifique se o server.py está rodando.")
