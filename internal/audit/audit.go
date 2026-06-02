// Package audit oferece leitura da trilha de auditoria (SOX Seção 802).
// A escrita da trilha é feita pelo próprio banco, via triggers definidos na
// migração 0002 — garantindo que NENHUMA operação na aplicação escape ao
// registro, mesmo que feita fora do código Go.
package audit

import (
	"context"
	"database/sql"
)

// Registro é uma linha da trilha de auditoria.
type Registro struct {
	Tabela     string
	Operacao   string
	RegistroID string
	Usuario    string
	OcorridoEm string
}

// Resumo conta as entradas por tabela/operação.
type Resumo struct {
	Tabela   string
	Operacao string
	Total    int
}

// ResumoTrilha agrega a trilha por tabela e operação.
func ResumoTrilha(ctx context.Context, conn *sql.DB) ([]Resumo, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT tabela, operacao, COUNT(*)
		  FROM auditoria
		 GROUP BY tabela, operacao
		 ORDER BY tabela, operacao`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Resumo
	for rows.Next() {
		var r Resumo
		if err := rows.Scan(&r.Tabela, &r.Operacao, &r.Total); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Total devolve a contagem total de registros na trilha.
func Total(ctx context.Context, conn *sql.DB) (int, error) {
	var n int
	err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM auditoria`).Scan(&n)
	return n, err
}
