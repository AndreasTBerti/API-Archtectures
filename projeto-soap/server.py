from spyne import Application, rcp, ServiceBase, Unicode, Integer
from spyne.protocol.soap import Soap11
from spyne.server.wsgi import WsgiApplication
from wsgiref.simple_server import make_server

prontuarios = {
    1: {"paciente": "José", "status": "doente"}
}

class ProntuarioService(ServiceBase):

    @rcp(Integer, _returns=Unicode)
    def get_paciente(ctx, id_paciente):
        return prontuarios[id_paciente]