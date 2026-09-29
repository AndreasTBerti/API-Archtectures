from spyne import Application, rpc, ServiceBase, Unicode, Integer, ComplexModel, Array
from spyne.model.fault import Fault
from spyne.protocol.soap import Soap11
from spyne.server.wsgi import WsgiApplication
import logging
import os
from wsgiref.simple_server import make_server

import banco

# O ProntuarioService é o dono dos dados do paciente: cadastro, consulta,
# alteração e remoção passam por aqui e ficam gravados em SQLite (banco.py).
# O projeto-rest guarda só os documentos gerados em cada vinda do paciente.

TAMANHO_MAX_NOME = 100
TAMANHO_MAX_STATUS = 50


class _RecusaSemTraceback(logging.Filter):
    """O Spyne registra todo Fault com traceback, inclusive os do cliente
    (Client.*), que são recusas esperadas, como um nome vazio. Aqui eles viram
    uma linha só no console; erros do servidor continuam com traceback."""

    def filter(self, record):
        erro = record.msg
        if isinstance(erro, Fault):
            record.msg, record.args = "Requisição recusada (%s): %s", (erro.faultcode, erro.faultstring)
        record.exc_info = record.exc_text = None
        return True


_log_recusas = logging.getLogger("spyne.application.client")
_handler = logging.StreamHandler()
_handler.addFilter(_RecusaSemTraceback())
_log_recusas.addHandler(_handler)
_log_recusas.propagate = False


class ProntuarioModel(ComplexModel):
    __namespace__ = "clinica.soap"

    id_paciente = Integer
    paciente = Unicode
    status = Unicode
    criado_em = Unicode
    atualizado_em = Unicode
    mensagem_erro = Unicode


NAO_ENCONTRADO = "Paciente não encontrado"


def _to_model(dados):
    return ProntuarioModel(
        id_paciente=dados["id_paciente"],
        paciente=dados["paciente"],
        status=dados["status"],
        criado_em=dados["criado_em"],
        atualizado_em=dados["atualizado_em"],
    )


def _validar(paciente, status):
    """Normaliza e valida os campos. Erro de validação vira um SOAP Fault do cliente."""
    paciente = (paciente or "").strip()
    status = (status or "").strip()
    if not paciente:
        raise Fault(faultcode="Client.Validacao", faultstring="O nome do paciente é obrigatório")
    if len(paciente) > TAMANHO_MAX_NOME:
        raise Fault(faultcode="Client.Validacao", faultstring=f"O nome do paciente passa de {TAMANHO_MAX_NOME} caracteres")
    if len(status) > TAMANHO_MAX_STATUS:
        raise Fault(faultcode="Client.Validacao", faultstring=f"O status passa de {TAMANHO_MAX_STATUS} caracteres")
    return paciente, status


class ProntuarioService(ServiceBase):

    @rpc(Integer, _returns=ProntuarioModel)
    def get_paciente(ctx, id_paciente):
        dados = banco.buscar(id_paciente) if id_paciente is not None else None
        return _to_model(dados) if dados else ProntuarioModel(mensagem_erro=NAO_ENCONTRADO)

    @rpc(_returns=Array(ProntuarioModel))
    def list_pacientes(ctx):
        return [_to_model(d) for d in banco.listar()]

    @rpc(Unicode, Unicode, _returns=ProntuarioModel)
    def criar_paciente(ctx, paciente, status):
        paciente, status = _validar(paciente, status)
        return _to_model(banco.criar(paciente, status))

    @rpc(Integer, Unicode, Unicode, _returns=ProntuarioModel)
    def atualizar_paciente(ctx, id_paciente, paciente, status):
        paciente, status = _validar(paciente, status)
        dados = banco.atualizar(id_paciente, paciente, status) if id_paciente is not None else None
        return _to_model(dados) if dados else ProntuarioModel(mensagem_erro=NAO_ENCONTRADO)

    @rpc(Integer, _returns=ProntuarioModel)
    def remover_paciente(ctx, id_paciente):
        dados = banco.remover(id_paciente) if id_paciente is not None else None
        return _to_model(dados) if dados else ProntuarioModel(mensagem_erro=NAO_ENCONTRADO)


application = Application(
    [ProntuarioService],
    tns='clinica.soap',
    in_protocol=Soap11(validator='lxml'),
    out_protocol=Soap11()
)

wsgi_app = WsgiApplication(application)

if __name__ == '__main__':
    porta = int(os.environ.get("SOAP_PORT", "8000"))
    print(f"Servidor SOAP em execução na porta {porta}...")
    print(f"Acesse o WSDL em: http://127.0.0.1:{porta}/?wsdl")
    print("Banco de dados:", banco.caminho_banco())
    server = make_server('127.0.0.1', porta, wsgi_app)
    server.serve_forever()
