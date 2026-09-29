package repository

import (
	"projeto-rest/internals/model"

	"gorm.io/gorm"
)

type PassagemRepository interface {
	// FindQuadro devolve as passagens abertas (de qualquer dia) e as do dia
	// informado que já tiveram alta ("Atendidos hoje").
	FindQuadro(hoje string) ([]model.Passagem, error)
	FindById(id uint) (*model.Passagem, error)
	// FindByPaciente lista as passagens do paciente, da mais recente para a mais antiga.
	FindByPaciente(pacienteID uint) ([]model.Passagem, error)
	// FindAberta devolve a passagem aberta do paciente, se houver.
	FindAberta(pacienteID uint) (*model.Passagem, error)
	FindByPacienteData(pacienteID uint, data string) (*model.Passagem, error)
	// FindAnteriores devolve as passagens encerradas do paciente com data
	// anterior a antesDe, da mais recente para a mais antiga. É a base do
	// resumo: a passagem atual nunca entra.
	FindAnteriores(pacienteID uint, antesDe string) ([]model.Passagem, error)
	CountByEtapa(etapa string) (int64, error)

	Create(p *model.Passagem) error
	// Update grava só os campos informados (ex.: etapa, pos_x, alta_em).
	Update(id uint, campos map[string]any) error
	UpdatePosicao(id uint, x, y float64) error

	// DeleteByPaciente remove as passagens do paciente com os documentos e
	// movimentações, e devolve os IDs removidos.
	DeleteByPaciente(pacienteID uint) ([]uint, error)

	UpsertAdmissao(a model.Admissao) error
	UpsertTriagem(t model.Triagem) error
	UpsertConsulta(c model.Consulta) error
}

type gormPassagemRepository struct{ db *gorm.DB }

func NewPassagemRepository(db *gorm.DB) PassagemRepository { return &gormPassagemRepository{db} }

func (r *gormPassagemRepository) query() *gorm.DB {
	return r.db.Preload("Admissao").Preload("Triagem").Preload("Consulta").
		Preload("Comentarios", func(db *gorm.DB) *gorm.DB {
			return db.Where("comentario <> '' AND finalizacao_id IS NOT NULL").Order("id ASC")
		})
}

// preencherAnteriores conta, para cada passagem, quantas o paciente teve antes dela.
func (r *gormPassagemRepository) preencherAnteriores(ps []model.Passagem) {
	for i := range ps {
		var n int64
		r.db.Model(&model.Passagem{}).
			Where("paciente_id = ? AND data < ?", ps[i].PacienteID, ps[i].Data).
			Count(&n)
		ps[i].Anteriores = int(n)
	}
}

func (r *gormPassagemRepository) lista(q *gorm.DB) ([]model.Passagem, error) {
	var ps []model.Passagem
	if err := q.Find(&ps).Error; err != nil {
		return nil, err
	}
	r.preencherAnteriores(ps)
	return ps, nil
}

func (r *gormPassagemRepository) uma(q *gorm.DB) (*model.Passagem, error) {
	var p model.Passagem
	if err := q.First(&p).Error; err != nil {
		return nil, err
	}
	ps := []model.Passagem{p}
	r.preencherAnteriores(ps)
	return &ps[0], nil
}

func (r *gormPassagemRepository) FindQuadro(hoje string) ([]model.Passagem, error) {
	return r.lista(r.query().Where("alta_em IS NULL OR data = ?", hoje).Order("chegada_em ASC"))
}

func (r *gormPassagemRepository) FindById(id uint) (*model.Passagem, error) {
	return r.uma(r.query().Where("id = ?", id))
}

func (r *gormPassagemRepository) FindByPaciente(pacienteID uint) ([]model.Passagem, error) {
	return r.lista(r.query().Where("paciente_id = ?", pacienteID).Order("data DESC"))
}

func (r *gormPassagemRepository) FindAberta(pacienteID uint) (*model.Passagem, error) {
	return r.uma(r.query().Where("paciente_id = ? AND alta_em IS NULL", pacienteID).Order("data DESC"))
}

func (r *gormPassagemRepository) FindByPacienteData(pacienteID uint, data string) (*model.Passagem, error) {
	return r.uma(r.query().Where("paciente_id = ? AND data = ?", pacienteID, data))
}

func (r *gormPassagemRepository) FindAnteriores(pacienteID uint, antesDe string) ([]model.Passagem, error) {
	return r.lista(r.query().
		Where("paciente_id = ? AND alta_em IS NOT NULL AND data < ?", pacienteID, antesDe).
		Order("data DESC"))
}

func (r *gormPassagemRepository) CountByEtapa(etapa string) (int64, error) {
	var n int64
	err := r.db.Model(&model.Passagem{}).Where("etapa = ? AND alta_em IS NULL", etapa).Count(&n).Error
	return n, err
}

func (r *gormPassagemRepository) Create(p *model.Passagem) error {
	p.Paciente, p.Admissao, p.Triagem, p.Consulta, p.Comentarios = nil, nil, nil, nil, nil
	return r.db.Create(p).Error
}

func (r *gormPassagemRepository) Update(id uint, campos map[string]any) error {
	return r.db.Model(&model.Passagem{}).Where("id = ?", id).Updates(campos).Error
}

func (r *gormPassagemRepository) UpdatePosicao(id uint, x, y float64) error {
	return r.Update(id, map[string]any{"pos_x": x, "pos_y": y})
}

func (r *gormPassagemRepository) DeleteByPaciente(pacienteID uint) ([]uint, error) {
	var ids []uint
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Passagem{}).Where("paciente_id = ?", pacienteID).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		for _, m := range []any{&model.Admissao{}, &model.Triagem{}, &model.Consulta{}, &model.Movimentacao{}} {
			if err := tx.Where("passagem_id IN ?", ids).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Where("paciente_id = ?", pacienteID).Delete(&model.Passagem{}).Error
	})
	return ids, err
}

// Os Upsert gravam o documento da passagem, mantendo ID e data de criação se
// ele já existir (edição de um documento da mesma passagem).

func (r *gormPassagemRepository) UpsertAdmissao(a model.Admissao) error {
	var existing model.Admissao
	if err := r.db.Where("passagem_id = ?", a.PassagemID).First(&existing).Error; err == nil {
		a.ID, a.CreatedAt = existing.ID, existing.CreatedAt
	}
	return r.db.Save(&a).Error
}

func (r *gormPassagemRepository) UpsertTriagem(t model.Triagem) error {
	var existing model.Triagem
	if err := r.db.Where("passagem_id = ?", t.PassagemID).First(&existing).Error; err == nil {
		t.ID, t.CreatedAt = existing.ID, existing.CreatedAt
	}
	return r.db.Save(&t).Error
}

func (r *gormPassagemRepository) UpsertConsulta(c model.Consulta) error {
	var existing model.Consulta
	if err := r.db.Where("passagem_id = ?", c.PassagemID).First(&existing).Error; err == nil {
		c.ID, c.CreatedAt = existing.ID, existing.CreatedAt
	}
	return r.db.Save(&c).Error
}
