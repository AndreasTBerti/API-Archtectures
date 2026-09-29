package repository

import (
	"projeto-rest/internals/model"

	"gorm.io/gorm"
)

type FinalizacaoRepository interface {
	// Registrar grava uma movimentação feita por um doutor.
	Registrar(m model.Movimentacao) error
	// Pendentes devolve as movimentações do doutor ainda não finalizadas, em ordem.
	Pendentes(doutorID uint) ([]model.Movimentacao, error)
	// Finalizar fecha o plantão: cria a finalização, liga as movimentações
	// pendentes a ela e grava os comentários (por ID da movimentação).
	Finalizar(f *model.Finalizacao, comentarios map[uint]string) error
	ListarDoDoutor(doutorID uint) ([]model.Finalizacao, error)
	FindById(id uint) (*model.Finalizacao, error)
}

type gormFinalizacaoRepository struct{ db *gorm.DB }

func NewFinalizacaoRepository(db *gorm.DB) FinalizacaoRepository {
	return &gormFinalizacaoRepository{db}
}

func (r *gormFinalizacaoRepository) Registrar(m model.Movimentacao) error {
	return r.db.Create(&m).Error
}

func (r *gormFinalizacaoRepository) Pendentes(doutorID uint) ([]model.Movimentacao, error) {
	var ms []model.Movimentacao
	err := r.db.Where("doutor_id = ? AND finalizacao_id IS NULL", doutorID).Order("id ASC").Find(&ms).Error
	return ms, err
}

func (r *gormFinalizacaoRepository) Finalizar(f *model.Finalizacao, comentarios map[uint]string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		f.Movimentacoes = nil
		if err := tx.Create(f).Error; err != nil {
			return err
		}
		// Só as pendentes do próprio doutor entram, mesmo que o corpo cite outras.
		var ids []uint
		if err := tx.Model(&model.Movimentacao{}).
			Where("doutor_id = ? AND finalizacao_id IS NULL AND created_at <= ?", f.DoutorID, f.Fim).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := tx.Model(&model.Movimentacao{}).Where("id IN ?", ids).
				Update("finalizacao_id", f.ID).Error; err != nil {
				return err
			}
		}
		for _, id := range ids {
			if c, ok := comentarios[id]; ok && c != "" {
				if err := tx.Model(&model.Movimentacao{}).Where("id = ?", id).Update("comentario", c).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *gormFinalizacaoRepository) query() *gorm.DB {
	return r.db.Preload("Movimentacoes", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") })
}

func (r *gormFinalizacaoRepository) ListarDoDoutor(doutorID uint) ([]model.Finalizacao, error) {
	var fs []model.Finalizacao
	err := r.query().Where("doutor_id = ?", doutorID).Order("fim DESC").Find(&fs).Error
	return fs, err
}

func (r *gormFinalizacaoRepository) FindById(id uint) (*model.Finalizacao, error) {
	var f model.Finalizacao
	if err := r.query().First(&f, id).Error; err != nil {
		return nil, err
	}
	return &f, nil
}
