package main

import (
	"log"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"projeto-rest/internals/model"
)

// migrar cria/atualiza as tabelas e converte bancos de versões anteriores.
//
// Os pacientes agora moram no serviço SOAP. O REST guarda só as passagens,
// os documentos e as finalizações de plantão, ligados ao paciente pelo
// id_paciente do SOAP. Bancos antigos passam por duas conversões:
//
//  1. Documentos no formato antigo (um de cada tipo por paciente) viram uma
//     passagem por paciente, datada pelo primeiro documento.
//  2. A tabela local de pacientes some: o paciente_id das passagens passa a
//     ser o soap_id que ela guardava.
func migrar(db *gorm.DB) error {
	legado, err := separarTabelasLegadas(db)
	if err != nil {
		return err
	}
	semFK, err := separarPassagensComFKPaciente(db)
	if err != nil {
		return err
	}

	if err := db.AutoMigrate(&model.User{}, &model.Passagem{}, &model.Admissao{}, &model.Triagem{}, &model.Consulta{},
		&model.Finalizacao{}, &model.Movimentacao{}); err != nil {
		return err
	}
	if semFK {
		if err := restaurarPassagens(db); err != nil {
			return err
		}
	}
	if len(legado) > 0 {
		if err := converterDocumentosLegados(db, legado); err != nil {
			return err
		}
	}
	return moverPacientesParaSoap(db)
}

// tabelaPacientesLocal devolve o nome da antiga tabela local de pacientes, se
// ela ainda existir ("pacientes_old" é de uma migração ainda mais antiga).
func tabelaPacientesLocal(db *gorm.DB) string {
	for _, t := range []string{"pacientes_old", "pacientes"} {
		if db.Migrator().HasTable(t) && db.Migrator().HasColumn(t, "soap_id") {
			return t
		}
	}
	return ""
}

// ddlDe devolve o CREATE TABLE de uma tabela.
func ddlDe(db *gorm.DB, tabela string) string {
	var ddl string
	db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", tabela).Scan(&ddl)
	return ddl
}

// renomearSemReescreverFKs renomeia a tabela sem que o SQLite reescreva as
// referências das outras tabelas para o nome novo (documentos continuam
// apontando para "passagens").
func renomearSemReescreverFKs(db *gorm.DB, de, para string) error {
	return db.Connection(func(conn *gorm.DB) error {
		if err := conn.Exec("PRAGMA legacy_alter_table = ON").Error; err != nil {
			return err
		}
		defer conn.Exec("PRAGMA legacy_alter_table = OFF")
		return conn.Exec("ALTER TABLE " + de + " RENAME TO " + para).Error
	})
}

// separarPassagensComFKPaciente: passagens criadas quando o REST ainda tinha
// pacientes têm chave estrangeira para a tabela local. A tabela é separada
// para o AutoMigrate recriá-la sem essa chave; restaurarPassagens copia as linhas.
func separarPassagensComFKPaciente(db *gorm.DB) (bool, error) {
	if !db.Migrator().HasTable("passagens") || !strings.Contains(ddlDe(db, "passagens"), "REFERENCES `pacientes`") {
		return false, nil
	}
	log.Println("Migrando passagens: removendo a chave estrangeira para a antiga tabela de pacientes")
	if db.Migrator().HasTable("passagens_fk") {
		if err := db.Exec("DROP TABLE passagens_fk").Error; err != nil {
			return false, err
		}
	}
	if err := removerIndices(db, "passagens"); err != nil {
		return false, err
	}
	return true, renomearSemReescreverFKs(db, "passagens", "passagens_fk")
}

func restaurarPassagens(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		cols := "id, paciente_id, data, etapa, pos_x, pos_y, chegada_em, alta_em, created_at, updated_at"
		if err := tx.Exec("INSERT INTO passagens (" + cols + ") SELECT " + cols + " FROM passagens_fk").Error; err != nil {
			return err
		}
		return tx.Exec("DROP TABLE passagens_fk").Error
	})
}

// moverPacientesParaSoap troca o id local do paciente pelo id do SOAP nas
// passagens e nas movimentações, e apaga a tabela local de pacientes.
func moverPacientesParaSoap(db *gorm.DB) error {
	t := tabelaPacientesLocal(db)
	if t == "" {
		return nil
	}
	log.Println("Migrando pacientes: os dados agora ficam no serviço SOAP; removendo a tabela local", t)
	return db.Transaction(func(tx *gorm.DB) error {
		for _, alvo := range []string{"passagens", "movimentacoes"} {
			if err := tx.Exec("UPDATE " + alvo + " SET paciente_id = (SELECT soap_id FROM " + t + " WHERE " + t + ".id = " + alvo + ".paciente_id) " +
				"WHERE paciente_id IN (SELECT id FROM " + t + ")").Error; err != nil {
				return err
			}
		}
		for _, tabela := range []string{"pacientes", "pacientes_old"} {
			if err := tx.Exec("DROP TABLE IF EXISTS " + tabela).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

const sufixoLegado = "_legado"

// separarTabelasLegadas renomeia as tabelas no formato antigo para que o
// AutoMigrate crie as novas. Devolve os nomes originais das tabelas renomeadas.
func separarTabelasLegadas(db *gorm.DB) ([]string, error) {
	m := db.Migrator()
	colunaLegada := map[string]string{
		"passagens": "estado", // antigo histórico de mudanças de estado
		"admissoes": "paciente_id",
		"triagems":  "paciente_id",
		"consulta":  "paciente_id",
	}
	var renomeadas []string
	for _, tabela := range []string{"passagens", "admissoes", "triagems", "consulta"} {
		if !m.HasTable(tabela) || !m.HasColumn(tabela, colunaLegada[tabela]) {
			continue
		}
		if m.HasTable(tabela + sufixoLegado) {
			if err := db.Exec("DROP TABLE " + tabela + sufixoLegado).Error; err != nil {
				return nil, err
			}
		}
		if err := removerIndices(db, tabela); err != nil {
			return nil, err
		}
		if err := db.Exec("ALTER TABLE " + tabela + " RENAME TO " + tabela + sufixoLegado).Error; err != nil {
			return nil, err
		}
		renomeadas = append(renomeadas, tabela)
	}
	if len(renomeadas) > 0 {
		log.Println("Migrando para passagens: tabelas antigas", renomeadas)
	}
	return renomeadas, nil
}

func removerIndices(db *gorm.DB, tabela string) error {
	var indices []string
	db.Raw("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = ? AND sql IS NOT NULL", tabela).Scan(&indices)
	for _, idx := range indices {
		if err := db.Exec("DROP INDEX IF EXISTS " + idx).Error; err != nil {
			return err
		}
	}
	return nil
}

// Colunas copiadas de cada documento antigo (doutor_id vira autor_id).
var colunasDoc = map[string]string{
	"admissoes": "queixa_principal, forma_chegada, acompanhante, convenio, observacoes",
	"triagems":  "pressao_arterial, temperatura, frequencia_cardiaca, saturacao_o2, peso, altura, sintomas, prioridade, observacoes",
	"consulta":  "anamnese, diagnostico, prescricao, exames_solicitados, encaminhamento, retorno, observacoes",
}

func converterDocumentosLegados(db *gorm.DB, legado []string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		docs := []string{}
		for _, t := range legado {
			if t != "passagens" {
				docs = append(docs, t)
			}
		}

		// criado[tabela][paciente] = data de criação do documento antigo
		criado := map[string]map[uint]time.Time{}
		pacientes := map[uint]bool{}
		for _, t := range docs {
			var linhas []struct {
				PacienteID uint
				CreatedAt  time.Time
			}
			filtro := ""
			if local := tabelaPacientesLocal(tx); local != "" {
				filtro = " WHERE paciente_id IN (SELECT id FROM " + local + ")"
			}
			if err := tx.Raw("SELECT paciente_id, created_at FROM " + t + sufixoLegado + filtro).Scan(&linhas).Error; err != nil {
				return err
			}
			criado[t] = map[uint]time.Time{}
			for _, l := range linhas {
				criado[t][l.PacienteID] = l.CreatedAt
				pacientes[l.PacienteID] = true
			}
		}

		ids := make([]uint, 0, len(pacientes))
		for id := range pacientes {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

		contagem := map[string]int64{}
		for _, pid := range ids {
			// A chegada é o documento mais antigo (normalmente a admissão).
			var chegada time.Time
			for _, t := range docs {
				if c, ok := criado[t][pid]; ok && (chegada.IsZero() || c.Before(chegada)) {
					chegada = c
				}
			}
			pg := model.Passagem{PacienteID: pid, Data: model.DataDe(chegada), ChegadaEm: chegada}
			if alta, ok := criado["consulta"][pid]; ok {
				pg.Etapa, pg.AltaEm = model.EtapaAlta, &alta
			} else if _, ok := criado["triagems"][pid]; ok {
				pg.Etapa = model.EtapaAguardandoConsulta
			} else {
				pg.Etapa = model.EtapaAguardandoTriagem
			}
			if pg.Aberta() {
				pg.PosX, pg.PosY = model.PosicaoPadrao(pg.Etapa, contagem[pg.Etapa])
				contagem[pg.Etapa]++
			}
			if err := tx.Create(&pg).Error; err != nil {
				return err
			}
			for _, t := range docs {
				cols := colunasDoc[t]
				if err := tx.Exec("INSERT INTO "+t+" (passagem_id, autor_id, "+cols+", created_at, updated_at) "+
					"SELECT ?, doutor_id, "+cols+", created_at, updated_at FROM "+t+sufixoLegado+" WHERE paciente_id = ?",
					pg.ID, pid).Error; err != nil {
					return err
				}
			}
		}

		for _, t := range legado {
			if err := tx.Exec("DROP TABLE " + t + sufixoLegado).Error; err != nil {
				return err
			}
		}
		log.Printf("Migração para passagens: %d passagem(ns) criada(s) a partir dos documentos antigos", len(ids))
		return nil
	})
}
