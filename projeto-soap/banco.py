"""Persistência dos pacientes do ProntuarioService em SQLite.

O serviço SOAP é o dono dos dados do paciente. O banco fica em
prontuarios.db, ao lado deste arquivo, ou no caminho da variável SOAP_DB.
Cada operação abre a sua própria conexão, o que é seguro com o servidor WSGI.
"""
import os
import sqlite3
from contextlib import contextmanager
from datetime import datetime, timezone
from pathlib import Path

# Pacientes criados na primeira execução, quando o banco está vazio.
PACIENTES_INICIAIS = [
    (1, "José", "doente"),
    (2, "Roberto", "saudável"),
]

_ESQUEMA = """
CREATE TABLE IF NOT EXISTS pacientes (
    id_paciente   INTEGER PRIMARY KEY AUTOINCREMENT,
    paciente      TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT '',
    criado_em     TEXT NOT NULL,
    atualizado_em TEXT NOT NULL
)
"""


def caminho_banco() -> str:
    return os.environ.get("SOAP_DB") or str(Path(__file__).with_name("prontuarios.db"))


def _agora() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


_inicializados: set[str] = set()


@contextmanager
def conexao():
    caminho = caminho_banco()
    con = sqlite3.connect(caminho)
    con.row_factory = sqlite3.Row
    try:
        if caminho not in _inicializados:
            _inicializar(con)
            _inicializados.add(caminho)
        yield con
        con.commit()
    except Exception:
        con.rollback()
        raise
    finally:
        con.close()


def _inicializar(con: sqlite3.Connection) -> None:
    con.execute(_ESQUEMA)
    vazio = con.execute("SELECT COUNT(*) FROM pacientes").fetchone()[0] == 0
    if vazio:
        agora = _agora()
        con.executemany(
            "INSERT INTO pacientes (id_paciente, paciente, status, criado_em, atualizado_em) VALUES (?, ?, ?, ?, ?)",
            [(i, nome, status, agora, agora) for i, nome, status in PACIENTES_INICIAIS],
        )


def buscar(id_paciente: int) -> dict | None:
    with conexao() as con:
        linha = con.execute("SELECT * FROM pacientes WHERE id_paciente = ?", (id_paciente,)).fetchone()
        return dict(linha) if linha else None


def listar() -> list[dict]:
    with conexao() as con:
        return [dict(l) for l in con.execute("SELECT * FROM pacientes ORDER BY paciente COLLATE NOCASE, id_paciente")]


def criar(paciente: str, status: str) -> dict:
    agora = _agora()
    with conexao() as con:
        cur = con.execute(
            "INSERT INTO pacientes (paciente, status, criado_em, atualizado_em) VALUES (?, ?, ?, ?)",
            (paciente, status, agora, agora),
        )
        novo_id = cur.lastrowid
    return buscar(novo_id)


def atualizar(id_paciente: int, paciente: str, status: str) -> dict | None:
    with conexao() as con:
        cur = con.execute(
            "UPDATE pacientes SET paciente = ?, status = ?, atualizado_em = ? WHERE id_paciente = ?",
            (paciente, status, _agora(), id_paciente),
        )
        if cur.rowcount == 0:
            return None
    return buscar(id_paciente)


def remover(id_paciente: int) -> dict | None:
    dados = buscar(id_paciente)
    if dados is None:
        return None
    with conexao() as con:
        con.execute("DELETE FROM pacientes WHERE id_paciente = ?", (id_paciente,))
    return dados
