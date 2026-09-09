import streamlit as st
from zeep import Client

st.set_page_config(page_title="Painel Prontuário SOAP", layout="centered")
st.title("Consulta de Prontuário Médico 🏥")

WSDL_URL = 'http://127.0.0.1:8000/?wsdl'

@st.cache_resource
def get_soap_client():
    try:
        return Client(wsdl=WSDL_URL)
    except Exception as e:
        st.error(f"Erro ao conectar com o servidor SOAP: {e}")
        return None

client = get_soap_client()

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
            
            # Exibição dos atributos do objeto
            col1, col2 = st.columns(2)
            with col1:
                st.metric(label="Paciente", value=resposta.paciente)
            with col2:
                st.metric(label="Status", value=resposta.status)
else:
    st.error("Servidor SOAP indisponível. Verifique se o server.py está rodando.")