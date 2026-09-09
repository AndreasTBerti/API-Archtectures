from spyne import Application, rpc, ServiceBase, Unicode, Integer, ComplexModel
from spyne.protocol.soap import Soap11
from spyne.server.wsgi import WsgiApplication
from wsgiref.simple_server import make_server

class ProntuarioModel(ComplexModel):
    paciente = Unicode
    status = Unicode
    mensagem_erro = Unicode

prontuarios = {
    1: {"paciente": "José", "status": "doente"},
    2: {"paciente": "Roberto", "status": "saudável"}
}

class ProntuarioService(ServiceBase):

    @rpc(Integer, _returns=ProntuarioModel)
    def get_paciente(ctx, id_paciente):
        dados = prontuarios.get(id_paciente)
        
        if dados:
            return ProntuarioModel(paciente=dados["paciente"], status=dados["status"])
        else:
            return ProntuarioModel(mensagem_erro="Paciente não encontrado")

application = Application(
    [ProntuarioService],
    tns='clinica.soap',
    in_protocol=Soap11(validator='lxml'),
    out_protocol=Soap11()
)

wsgi_app = WsgiApplication(application)

if __name__ == '__main__':
    print("Servidor SOAP em execução na porta 8000...")
    print("Acesse o WSDL em: http://127.0.0.1:8000/?wsdl")
    server = make_server('127.0.0.1', 8000, wsgi_app)
    server.serve_forever()